package persistence

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// TradeRecord represents a single trade in the journal.
type TradeRecord struct {
	ID        int64     `json:"id"`
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"` // "LONG" or "SHORT"
	EntryPrice float64  `json:"entry_price"`
	ExitPrice  float64  `json:"exit_price"`
	Quantity   float64  `json:"quantity"`
	PnL        float64  `json:"pnl"`
	PnLPct     float64  `json:"pnl_pct"`
	RMultiple  float64  `json:"r_multiple"`
	SetupType  string   `json:"setup_type"`
	Notes      string   `json:"notes"`
	EntryTime  time.Time `json:"entry_time"`
	ExitTime   time.Time `json:"exit_time"`
	Exchange   string   `json:"exchange"`
	Fees       float64  `json:"fees"`
	Paper      bool     `json:"paper"`
}

// DailySummary holds aggregated stats for a single day.
type DailySummary struct {
	Date        time.Time `json:"date"`
	TotalPnL    float64   `json:"total_pnl"`
	WinCount    int       `json:"win_count"`
	LossCount   int       `json:"loss_count"`
	TotalTrades int       `json:"total_trades"`
	MaxWin      float64   `json:"max_win"`
	MaxLoss     float64   `json:"max_loss"`
	AvgR        float64   `json:"avg_r"`
}

// PerformanceStats holds overall performance metrics.
type PerformanceStats struct {
	TotalPnL     float64 `json:"total_pnl"`
	WinRate      float64 `json:"win_rate"`
	AvgWinner    float64 `json:"avg_winner"`
	AvgLoser     float64 `json:"avg_loser"`
	ProfitFactor float64 `json:"profit_factor"`
	Expectancy   float64 `json:"expectancy"`
	SharpeApprox float64 `json:"sharpe_approx"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	BestDay      float64 `json:"best_day"`
	WorstDay     float64 `json:"worst_day"`
	TotalTrades  int     `json:"total_trades"`
}

// DB wraps the SQLite database for the trade journal.
type DB struct {
	db *sql.DB
}

// NewDB opens or creates the SQLite database at the given path.
func NewDB(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("persistence: creating db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("persistence: opening database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("persistence: setting WAL mode: %w", err)
	}

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("persistence: migration: %w", err)
	}

	return d, nil
}

func (d *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS trades (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol TEXT NOT NULL,
			side TEXT NOT NULL,
			entry_price REAL NOT NULL,
			exit_price REAL NOT NULL,
			quantity REAL NOT NULL,
			pnl REAL NOT NULL,
			pnl_pct REAL NOT NULL,
			r_multiple REAL NOT NULL DEFAULT 0,
			setup_type TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			entry_time DATETIME NOT NULL,
			exit_time DATETIME NOT NULL,
			exchange TEXT NOT NULL DEFAULT '',
			fees REAL NOT NULL DEFAULT 0,
			paper INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS daily_summary (
			date TEXT PRIMARY KEY,
			total_pnl REAL NOT NULL DEFAULT 0,
			win_count INTEGER NOT NULL DEFAULT 0,
			loss_count INTEGER NOT NULL DEFAULT 0,
			total_trades INTEGER NOT NULL DEFAULT 0,
			max_win REAL NOT NULL DEFAULT 0,
			max_loss REAL NOT NULL DEFAULT 0,
			avg_r REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol TEXT NOT NULL,
			condition TEXT NOT NULL,
			value REAL NOT NULL,
			triggered INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_trades_exit_time ON trades(exit_time)`,
		`CREATE INDEX IF NOT EXISTS idx_trades_paper ON trades(paper)`,
	}

	for _, m := range migrations {
		if _, err := d.db.Exec(m); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// LogTrade inserts a trade record and updates the daily summary.
func (d *DB) LogTrade(trade TradeRecord) error {
	paper := 0
	if trade.Paper {
		paper = 1
	}

	_, err := d.db.Exec(`INSERT INTO trades
		(symbol, side, entry_price, exit_price, quantity, pnl, pnl_pct, r_multiple,
		 setup_type, notes, entry_time, exit_time, exchange, fees, paper)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		trade.Symbol, trade.Side, trade.EntryPrice, trade.ExitPrice,
		trade.Quantity, trade.PnL, trade.PnLPct, trade.RMultiple,
		trade.SetupType, trade.Notes,
		trade.EntryTime.Format(time.RFC3339),
		trade.ExitTime.Format(time.RFC3339),
		trade.Exchange, trade.Fees, paper)
	if err != nil {
		return fmt.Errorf("persistence: inserting trade: %w", err)
	}

	// Update daily summary
	dateStr := trade.ExitTime.Format("2006-01-02")
	return d.updateDailySummary(dateStr)
}

func (d *DB) updateDailySummary(dateStr string) error {
	row := d.db.QueryRow(`SELECT
		COALESCE(SUM(pnl), 0),
		COALESCE(SUM(CASE WHEN pnl > 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN pnl <= 0 THEN 1 ELSE 0 END), 0),
		COUNT(*),
		COALESCE(MAX(pnl), 0),
		COALESCE(MIN(pnl), 0),
		COALESCE(AVG(r_multiple), 0)
		FROM trades WHERE SUBSTR(exit_time, 1, 10) = ?`, dateStr)

	var totalPnL, maxWin, maxLoss, avgR float64
	var winCount, lossCount, totalTrades int
	if err := row.Scan(&totalPnL, &winCount, &lossCount, &totalTrades, &maxWin, &maxLoss, &avgR); err != nil {
		return fmt.Errorf("persistence: computing daily summary: %w", err)
	}

	_, err := d.db.Exec(`INSERT INTO daily_summary (date, total_pnl, win_count, loss_count, total_trades, max_win, max_loss, avg_r)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			total_pnl=excluded.total_pnl,
			win_count=excluded.win_count,
			loss_count=excluded.loss_count,
			total_trades=excluded.total_trades,
			max_win=excluded.max_win,
			max_loss=excluded.max_loss,
			avg_r=excluded.avg_r`,
		dateStr, totalPnL, winCount, lossCount, totalTrades, maxWin, maxLoss, avgR)
	return err
}

// GetTrades returns trades within a time range.
func (d *DB) GetTrades(from, to time.Time) ([]TradeRecord, error) {
	rows, err := d.db.Query(`SELECT id, symbol, side, entry_price, exit_price, quantity,
		pnl, pnl_pct, r_multiple, setup_type, notes, entry_time, exit_time, exchange, fees, paper
		FROM trades WHERE exit_time >= ? AND exit_time <= ? ORDER BY exit_time DESC`,
		from.Format(time.RFC3339), to.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("persistence: querying trades: %w", err)
	}
	defer rows.Close()

	var trades []TradeRecord
	for rows.Next() {
		var t TradeRecord
		var paper int
		var entryTimeStr, exitTimeStr string
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &t.EntryPrice, &t.ExitPrice,
			&t.Quantity, &t.PnL, &t.PnLPct, &t.RMultiple, &t.SetupType, &t.Notes,
			&entryTimeStr, &exitTimeStr, &t.Exchange, &t.Fees, &paper); err != nil {
			return nil, fmt.Errorf("persistence: scanning trade: %w", err)
		}
		t.EntryTime, _ = time.Parse(time.RFC3339, entryTimeStr)
		t.ExitTime, _ = time.Parse(time.RFC3339, exitTimeStr)
		t.Paper = paper != 0
		trades = append(trades, t)
	}
	return trades, rows.Err()
}

// GetDailySummary returns the summary for a specific date.
func (d *DB) GetDailySummary(date time.Time) (DailySummary, error) {
	dateStr := date.Format("2006-01-02")
	row := d.db.QueryRow(`SELECT date, total_pnl, win_count, loss_count, total_trades, max_win, max_loss, avg_r
		FROM daily_summary WHERE date = ?`, dateStr)

	var ds DailySummary
	var dateS string
	err := row.Scan(&dateS, &ds.TotalPnL, &ds.WinCount, &ds.LossCount, &ds.TotalTrades, &ds.MaxWin, &ds.MaxLoss, &ds.AvgR)
	if err == sql.ErrNoRows {
		return DailySummary{Date: date}, nil
	}
	if err != nil {
		return ds, fmt.Errorf("persistence: getting daily summary: %w", err)
	}
	ds.Date, _ = time.Parse("2006-01-02", dateS)
	return ds, nil
}

// GetPerformanceStats computes aggregate performance metrics over the last N days.
func (d *DB) GetPerformanceStats(days int) (PerformanceStats, error) {
	since := time.Now().AddDate(0, 0, -days)
	sinceStr := since.Format(time.RFC3339)
	var ps PerformanceStats

	// Basic aggregates
	row := d.db.QueryRow(`SELECT
		COALESCE(SUM(pnl), 0),
		COUNT(*),
		COALESCE(SUM(CASE WHEN pnl > 0 THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN pnl > 0 THEN pnl END), 0),
		COALESCE(AVG(CASE WHEN pnl <= 0 THEN pnl END), 0),
		COALESCE(SUM(CASE WHEN pnl > 0 THEN pnl ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN pnl < 0 THEN ABS(pnl) ELSE 0 END), 0)
		FROM trades WHERE exit_time >= ?`, sinceStr)

	var winCount int
	var grossWin, grossLoss float64
	if err := row.Scan(&ps.TotalPnL, &ps.TotalTrades, &winCount, &ps.AvgWinner,
		&ps.AvgLoser, &grossWin, &grossLoss); err != nil {
		return ps, fmt.Errorf("persistence: getting performance stats: %w", err)
	}

	if ps.TotalTrades > 0 {
		ps.WinRate = float64(winCount) / float64(ps.TotalTrades) * 100
	}
	if grossLoss > 0 {
		ps.ProfitFactor = grossWin / grossLoss
	}
	if ps.TotalTrades > 0 {
		ps.Expectancy = ps.TotalPnL / float64(ps.TotalTrades)
	}

	// Daily PnL for Sharpe and drawdown
	rows, err := d.db.Query(`SELECT total_pnl FROM daily_summary WHERE date >= ? ORDER BY date ASC`,
		since.Format("2006-01-02"))
	if err != nil {
		return ps, fmt.Errorf("persistence: querying daily pnl: %w", err)
	}
	defer rows.Close()

	var dailyPnLs []float64
	for rows.Next() {
		var pnl float64
		if err := rows.Scan(&pnl); err != nil {
			return ps, err
		}
		dailyPnLs = append(dailyPnLs, pnl)
	}

	if len(dailyPnLs) > 1 {
		// Approximate Sharpe: mean / stddev * sqrt(252)
		var sum, sumSq float64
		for _, p := range dailyPnLs {
			sum += p
			sumSq += p * p
		}
		n := float64(len(dailyPnLs))
		mean := sum / n
		variance := sumSq/n - mean*mean
		if variance > 0 {
			ps.SharpeApprox = (mean / math.Sqrt(variance)) * math.Sqrt(252)
		}

		// Best/worst day
		ps.BestDay = dailyPnLs[0]
		ps.WorstDay = dailyPnLs[0]
		for _, p := range dailyPnLs {
			if p > ps.BestDay {
				ps.BestDay = p
			}
			if p < ps.WorstDay {
				ps.WorstDay = p
			}
		}

		// Max drawdown from equity curve
		peak := 0.0
		equity := 0.0
		for _, p := range dailyPnLs {
			equity += p
			if equity > peak {
				peak = equity
			}
			dd := peak - equity
			if dd > ps.MaxDrawdown {
				ps.MaxDrawdown = dd
			}
		}
	}

	return ps, nil
}

// SaveSetting stores a key-value pair.
func (d *DB) SaveSetting(key, value string) error {
	_, err := d.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetSetting retrieves a setting value by key.
func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// UpdateTradeNotes updates the notes for a specific trade.
func (d *DB) UpdateTradeNotes(id int64, notes string) error {
	_, err := d.db.Exec(`UPDATE trades SET notes = ? WHERE id = ?`, notes, id)
	return err
}

// GetDailyEquityCurve returns the cumulative PnL per day over the last N days.
func (d *DB) GetDailyEquityCurve(days int) ([]float64, error) {
	since := time.Now().AddDate(0, 0, -days)
	rows, err := d.db.Query(`SELECT total_pnl FROM daily_summary WHERE date >= ? ORDER BY date ASC`,
		since.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var curve []float64
	cumulative := 0.0
	for rows.Next() {
		var pnl float64
		if err := rows.Scan(&pnl); err != nil {
			return nil, err
		}
		cumulative += pnl
		curve = append(curve, cumulative)
	}
	return curve, rows.Err()
}
