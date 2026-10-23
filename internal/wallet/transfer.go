package wallet

import (
	"database/sql"
	"errors"
	"math"
)

var ErrInsufficient = errors.New("insufficient wallet balance")
var ErrInvalidTransfer = errors.New("invalid transfer")
var ErrBalanceOverflow = errors.New("recipient balance overflow")

// Transfer commits debit and credit together. No compensating write is needed.
func Transfer(from, to int64, amount int) error { return sdb.transfer(from, to, amount) }

func (s *Storage) transfer(from, to int64, amount int) error {
	if from <= 0 || to <= 0 || amount <= 0 {
		return ErrInvalidTransfer
	}
	s.Lock()
	defer s.Unlock()
	return s.db.Transaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE storage SET money=money-? WHERE uid=? AND money>=?", amount, from, amount)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInsufficient
		}
		var balance int
		err = tx.QueryRow("SELECT money FROM storage WHERE uid=?", to).Scan(&balance)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if balance < 0 || balance > math.MaxInt-amount {
			return ErrBalanceOverflow
		}
		_, err = tx.Exec("INSERT INTO storage(uid,money) VALUES(?,?) ON CONFLICT(uid) DO UPDATE SET money=excluded.money", to, balance+amount)
		return err
	})
}
