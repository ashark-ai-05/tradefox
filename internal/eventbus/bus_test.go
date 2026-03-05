package eventbus

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestTopicBus_SubscribePublish(t *testing.T) {
	bus := NewTopicBus[string](testLogger())
	defer bus.Close()

	id, ch := bus.Subscribe(10)
	if id == 0 {
		t.Fatal("expected non-zero subscriber ID")
	}

	bus.Publish("hello")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Fatalf("expected 'hello', got %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestTopicBus_MultipleSubscribers(t *testing.T) {
	bus := NewTopicBus[int](testLogger())
	defer bus.Close()

	const numSubs = 3
	channels := make([]<-chan int, numSubs)
	for i := 0; i < numSubs; i++ {
		_, ch := bus.Subscribe(10)
		channels[i] = ch
	}

	bus.Publish(42)

	for i, ch := range channels {
		select {
		case val := <-ch:
			if val != 42 {
				t.Fatalf("subscriber %d: expected 42, got %d", i, val)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestTopicBus_Unsubscribe(t *testing.T) {
	bus := NewTopicBus[string](testLogger())
	defer bus.Close()

	id1, ch1 := bus.Subscribe(10)
	_, ch2 := bus.Subscribe(10)

	bus.Unsubscribe(id1)

	bus.Publish("after-unsub")

	// ch1 should be closed (unsubscribed)
	select {
	case _, ok := <-ch1:
		if ok {
			t.Fatal("expected ch1 to be closed after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from closed ch1")
	}

	// ch2 should receive the event
	select {
	case val := <-ch2:
		if val != "after-unsub" {
			t.Fatalf("expected 'after-unsub', got %q", val)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event on ch2")
	}
}

func TestTopicBus_PublishBatch(t *testing.T) {
	bus := NewTopicBus[int](testLogger())
	defer bus.Close()

	_, ch := bus.Subscribe(10)

	events := []int{1, 2, 3, 4, 5}
	bus.PublishBatch(events)

	for i, expected := range events {
		select {
		case val := <-ch:
			if val != expected {
				t.Fatalf("event %d: expected %d, got %d", i, expected, val)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestTopicBus_NonBlockingPublish(t *testing.T) {
	bus := NewTopicBus[int](testLogger())
	defer bus.Close()

	// Subscribe with buffer size 1 - most events will be dropped
	_, _ = bus.Subscribe(1)

	// Publish 100 events rapidly - this must not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			bus.Publish(i)
		}
		close(done)
	}()

	select {
	case <-done:
		// Success: publishing 100 events did not block
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked - non-blocking send is broken")
	}
}

func TestTopicBus_SubscriberCount(t *testing.T) {
	bus := NewTopicBus[int](testLogger())
	defer bus.Close()

	if bus.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers, got %d", bus.SubscriberCount())
	}

	id1, _ := bus.Subscribe(10)
	id2, _ := bus.Subscribe(10)
	_, _ = bus.Subscribe(10)

	if bus.SubscriberCount() != 3 {
		t.Fatalf("expected 3 subscribers, got %d", bus.SubscriberCount())
	}

	bus.Unsubscribe(id1)
	if bus.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers after unsubscribe, got %d", bus.SubscriberCount())
	}

	bus.Unsubscribe(id2)
	if bus.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber after second unsubscribe, got %d", bus.SubscriberCount())
	}
}

func TestTopicBus_Close(t *testing.T) {
	bus := NewTopicBus[string](testLogger())

	_, ch1 := bus.Subscribe(10)
	_, ch2 := bus.Subscribe(10)

	bus.Close()

	// Both channels should be closed
	for i, ch := range []<-chan string{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("channel %d: expected closed channel", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("channel %d: timed out reading from closed channel", i)
		}
	}

	if bus.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers after close, got %d", bus.SubscriberCount())
	}
}

func TestTopicBus_PublishAfterClose(t *testing.T) {
	bus := NewTopicBus[string](testLogger())
	bus.Close()

	// Should not panic
	bus.Publish("after-close")
	bus.PublishBatch([]string{"a", "b", "c"})
}

func TestTopicBus_ConcurrentPublishSubscribe(t *testing.T) {
	bus := NewTopicBus[int](testLogger())
	defer bus.Close()

	var wg sync.WaitGroup

	// 10 goroutines publishing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				bus.Publish(id*1000 + j)
			}
		}(i)
	}

	// 5 goroutines subscribing and unsubscribing
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				subID, ch := bus.Subscribe(5)
				// Drain a few events
				for k := 0; k < 3; k++ {
					select {
					case <-ch:
					case <-time.After(10 * time.Millisecond):
					}
				}
				bus.Unsubscribe(subID)
			}
		}()
	}

	wg.Wait()
}

func TestBus_AllTopics(t *testing.T) {
	bus := NewBus(testLogger())
	defer bus.Close()

	if bus.OrderBooks == nil {
		t.Fatal("OrderBooks topic bus is nil")
	}
	if bus.Trades == nil {
		t.Fatal("Trades topic bus is nil")
	}
	if bus.Providers == nil {
		t.Fatal("Providers topic bus is nil")
	}
	if bus.Positions == nil {
		t.Fatal("Positions topic bus is nil")
	}
	if bus.Studies == nil {
		t.Fatal("Studies topic bus is nil")
	}

	// Publish to each topic and verify receipt
	_, obCh := bus.OrderBooks.Subscribe(1)
	bus.OrderBooks.Publish(&models.OrderBook{Symbol: "BTCUSD"})
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTCUSD" {
			t.Fatalf("OrderBooks: expected symbol BTCUSD, got %s", ob.Symbol)
		}
	case <-time.After(time.Second):
		t.Fatal("OrderBooks: timed out")
	}

	_, tradeCh := bus.Trades.Subscribe(1)
	bus.Trades.Publish(models.Trade{Symbol: "ETHUSD"})
	select {
	case tr := <-tradeCh:
		if tr.Symbol != "ETHUSD" {
			t.Fatalf("Trades: expected symbol ETHUSD, got %s", tr.Symbol)
		}
	case <-time.After(time.Second):
		t.Fatal("Trades: timed out")
	}

	_, provCh := bus.Providers.Subscribe(1)
	bus.Providers.Publish(models.Provider{ProviderName: "binance"})
	select {
	case p := <-provCh:
		if p.ProviderName != "binance" {
			t.Fatalf("Providers: expected name binance, got %s", p.ProviderName)
		}
	case <-time.After(time.Second):
		t.Fatal("Providers: timed out")
	}

	_, posCh := bus.Positions.Subscribe(1)
	bus.Positions.Publish(models.Order{Symbol: "SOLUSD"})
	select {
	case o := <-posCh:
		if o.Symbol != "SOLUSD" {
			t.Fatalf("Positions: expected symbol SOLUSD, got %s", o.Symbol)
		}
	case <-time.After(time.Second):
		t.Fatal("Positions: timed out")
	}

	_, studyCh := bus.Studies.Subscribe(1)
	bus.Studies.Publish(models.BaseStudyModel{MarketMidPrice: 100.5})
	select {
	case s := <-studyCh:
		if s.MarketMidPrice != 100.5 {
			t.Fatalf("Studies: expected mid price 100.5, got %f", s.MarketMidPrice)
		}
	case <-time.After(time.Second):
		t.Fatal("Studies: timed out")
	}
}
