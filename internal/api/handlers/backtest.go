package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BacktestRunRequest is the JSON body for POST /api/backtest/run.
type BacktestRunRequest struct {
	Strategy       string            `json:"strategy"`
	Symbols        []string          `json:"symbols"`
	Venue          string            `json:"venue"`
	DataType       string            `json:"data_type"`
	StartNs        int64             `json:"start_ns"`
	EndNs          int64             `json:"end_ns"`
	StrategyParams map[string]string `json:"strategy_params"`
}

// BacktestRunResponse is returned by POST /api/backtest/run.
type BacktestRunResponse struct {
	ID           string             `json:"id"`
	Status       string             `json:"status"`
	TotalReturn  float64            `json:"total_return"`
	SharpeRatio  float64            `json:"sharpe_ratio"`
	MaxDrawdown  float64            `json:"max_drawdown"`
	WinRate      float64            `json:"win_rate"`
	ProfitFactor float64            `json:"profit_factor"`
	TotalTrades  int32              `json:"total_trades"`
	EquityCurve  []EquityCurvePoint `json:"equity_curve"`
	Trades       []TradeRecordJSON  `json:"trades"`
	ErrorMsg     string             `json:"error,omitempty"`
}

// EquityCurvePoint for JSON response.
type EquityCurvePoint struct {
	TimestampNs int64   `json:"timestamp_ns"`
	Equity      float64 `json:"equity"`
	Drawdown    float64 `json:"drawdown"`
}

// TradeRecordJSON for JSON response.
type TradeRecordJSON struct {
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	EntryPrice float64 `json:"entry_price"`
	ExitPrice  float64 `json:"exit_price"`
	Quantity   float64 `json:"quantity"`
	PnL        float64 `json:"pnl"`
	PnLPct     float64 `json:"pnl_pct"`
}

// DataImportRequest is the JSON body for POST /api/data/import.
type DataImportRequest struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	StartNs  int64  `json:"start_ns"`
	EndNs    int64  `json:"end_ns"`
}

func grpcConn(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// RunBacktest handles POST /api/backtest/run.
func RunBacktest(logger *slog.Logger, grpcAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BacktestRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
			return
		}

		conn, err := grpcConn(grpcAddr)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "nautilus not connected"})
			return
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		config := &pb.BacktestConfig{
			StrategyClass:  req.Strategy,
			StrategyParams: req.StrategyParams,
			Venue:          req.Venue,
			Symbols:        req.Symbols,
			StartNs:        req.StartNs,
			EndNs:          req.EndNs,
			DataType:       req.DataType,
		}
		if config.Venue == "" {
			config.Venue = "BINANCE"
		}
		if len(config.Symbols) == 0 {
			config.Symbols = []string{"BTCUSDT"}
		}
		if config.StrategyParams == nil {
			config.StrategyParams = make(map[string]string)
		}

		stream, err := client.RunBacktest(r.Context(), config)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		// Consume stream, return final result
		var lastProgress *pb.BacktestProgress
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			lastProgress = msg
		}

		resp := BacktestRunResponse{Status: "complete"}
		if lastProgress != nil {
			resp.ID = lastProgress.BacktestId
			if lastProgress.Status == "error" {
				resp.Status = "error"
				resp.ErrorMsg = lastProgress.Message
				writeJSON(w, http.StatusOK, resp)
				return
			}
		}

		// Fetch full result
		if resp.ID != "" {
			result, err := client.GetBacktestResult(r.Context(), &pb.BacktestId{Id: resp.ID})
			if err == nil {
				resp.TotalReturn = result.TotalReturn
				resp.SharpeRatio = result.SharpeRatio
				resp.MaxDrawdown = result.MaxDrawdown
				resp.WinRate = result.WinRate
				resp.ProfitFactor = result.ProfitFactor
				resp.TotalTrades = result.TotalTrades
				for _, p := range result.EquityCurve {
					resp.EquityCurve = append(resp.EquityCurve, EquityCurvePoint{
						TimestampNs: p.TimestampNs, Equity: p.Equity, Drawdown: p.Drawdown,
					})
				}
				for _, t := range result.Trades {
					resp.Trades = append(resp.Trades, TradeRecordJSON{
						Symbol: t.Symbol, Side: t.Side,
						EntryPrice: t.EntryPrice, ExitPrice: t.ExitPrice,
						Quantity: t.Quantity, PnL: t.Pnl, PnLPct: t.PnlPct,
					})
				}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// GetBacktestResult handles GET /api/backtest/{id}.
func GetBacktestResult(logger *slog.Logger, grpcAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing id"})
			return
		}

		conn, err := grpcConn(grpcAddr)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "nautilus not connected"})
			return
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		result, err := client.GetBacktestResult(r.Context(), &pb.BacktestId{Id: id})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		resp := BacktestRunResponse{
			ID: result.Id, Status: "complete",
			TotalReturn: result.TotalReturn, SharpeRatio: result.SharpeRatio,
			MaxDrawdown: result.MaxDrawdown, WinRate: result.WinRate,
			ProfitFactor: result.ProfitFactor, TotalTrades: result.TotalTrades,
		}
		for _, p := range result.EquityCurve {
			resp.EquityCurve = append(resp.EquityCurve, EquityCurvePoint{
				TimestampNs: p.TimestampNs, Equity: p.Equity, Drawdown: p.Drawdown,
			})
		}
		for _, t := range result.Trades {
			resp.Trades = append(resp.Trades, TradeRecordJSON{
				Symbol: t.Symbol, Side: t.Side,
				EntryPrice: t.EntryPrice, ExitPrice: t.ExitPrice,
				Quantity: t.Quantity, PnL: t.Pnl, PnLPct: t.PnlPct,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ListBacktests handles GET /api/backtest/list.
func ListBacktests(logger *slog.Logger, grpcAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := grpcConn(grpcAddr)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "nautilus not connected"})
			return
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		resp, err := client.ListBacktests(r.Context(), &pb.Empty{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		type summary struct {
			ID            string  `json:"id"`
			StrategyClass string  `json:"strategy_class"`
			Status        string  `json:"status"`
			TotalReturn   float64 `json:"total_return"`
			SharpeRatio   float64 `json:"sharpe_ratio"`
		}
		var items []summary
		for _, bt := range resp.Backtests {
			items = append(items, summary{
				ID: bt.Id, StrategyClass: bt.StrategyClass,
				Status: bt.Status, TotalReturn: bt.TotalReturn,
				SharpeRatio: bt.SharpeRatio,
			})
		}

		writeJSON(w, http.StatusOK, items)
	}
}

// GetStrategies handles GET /api/strategies.
func GetStrategies(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		strategies := []string{"scalp_absorption", "day_fvg", "swing_liquidity"}
		writeJSON(w, http.StatusOK, strategies)
	}
}

// ImportData handles POST /api/data/import.
func ImportData(logger *slog.Logger, grpcAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DataImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
			return
		}

		conn, err := grpcConn(grpcAddr)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "nautilus not connected"})
			return
		}
		defer conn.Close()

		client := pb.NewDataServiceClient(conn)
		importReq := &pb.ImportRequest{
			Venue:    "BINANCE",
			Symbol:   req.Symbol,
			DataType: req.Interval,
			StartNs:  req.StartNs,
			EndNs:    req.EndNs,
		}
		if importReq.Symbol == "" {
			importReq.Symbol = "BTCUSDT"
		}
		if importReq.DataType == "" {
			importReq.DataType = "1m"
		}

		stream, err := client.ImportData(r.Context(), importReq)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		type importResp struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Records int64  `json:"records_imported"`
		}

		var lastMsg *pb.ImportProgress
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			lastMsg = msg
		}

		resp := importResp{Status: "complete"}
		if lastMsg != nil {
			resp.Status = lastMsg.Status
			resp.Message = lastMsg.Message
			resp.Records = lastMsg.RecordsImported
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// GetDataAvailable handles GET /api/data/available.
func GetDataAvailable(logger *slog.Logger, grpcAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := grpcConn(grpcAddr)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "nautilus not connected"})
			return
		}
		defer conn.Close()

		client := pb.NewDataServiceClient(conn)
		resp, err := client.ListInstruments(r.Context(), &pb.InstrumentFilter{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		type inst struct {
			Symbol string `json:"symbol"`
			Venue  string `json:"venue"`
			Type   string `json:"instrument_type"`
		}
		var items []inst
		for _, i := range resp.Instruments {
			items = append(items, inst{
				Symbol: i.Symbol, Venue: i.Venue, Type: i.InstrumentType,
			})
		}

		writeJSON(w, http.StatusOK, items)
	}
}
