package wallet

import (
	"errors"
	sql "github.com/FloatTech/sqlite"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testStore(t *testing.T) *Storage {
	t.Helper()
	s := &Storage{db: sql.New(filepath.Join(t.TempDir(), "wallet.db"))}
	if err := s.db.Open(time.Minute); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	if _, err := s.db.Exec("CREATE TABLE storage(uid INTEGER PRIMARY KEY,money INTEGER); INSERT INTO storage VALUES(1,100),(2,0)"); err != nil {
		t.Fatal(err)
	}
	return s
}
func balances(t *testing.T, s *Storage, a, b int) {
	t.Helper()
	if x, y := s.getWalletOf(1).Money, s.getWalletOf(2).Money; x != a || y != b {
		t.Fatalf("balances %d/%d want %d/%d", x, y, a, b)
	}
}
func TestTransferCreditFailureRollsBack(t *testing.T) {
	s := testStore(t)
	_, err := s.db.Exec("CREATE TRIGGER fail_credit BEFORE INSERT ON storage WHEN NEW.uid=2 BEGIN SELECT RAISE(ABORT,'synthetic failure');END;")
	if err != nil {
		t.Fatal(err)
	}
	if s.transfer(1, 2, 10) == nil {
		t.Fatal("expected error")
	}
	balances(t, s, 100, 0)
	if _, err = s.db.Exec("DROP TRIGGER fail_credit"); err != nil {
		t.Fatal(err)
	}
	if err = s.transfer(1, 2, 10); err != nil {
		t.Fatal(err)
	}
	balances(t, s, 90, 10)
}
func TestConcurrentTransfersCannotOverspend(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.transfer(1, 2, 10)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrInsufficient) {
				t.Errorf("unexpected %v", err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 10 {
		t.Fatal(successes.Load())
	}
	balances(t, s, 0, 100)
}
func TestTransferValidationAndSelf(t *testing.T) {
	s := testStore(t)
	for _, a := range []struct {
		from, to int64
		amount   int
	}{{0, 2, 1}, {1, 0, 1}, {1, 2, 0}, {1, 2, -1}} {
		if !errors.Is(s.transfer(a.from, a.to, a.amount), ErrInvalidTransfer) {
			t.Fatal(a)
		}
	}
	if !errors.Is(s.transfer(1, 2, 101), ErrInsufficient) {
		t.Fatal("overspend")
	}
	if err := s.transfer(1, 1, 100); err != nil {
		t.Fatal(err)
	}
	balances(t, s, 100, 0)
}
func TestTransferOverflowRollback(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec("UPDATE storage SET money=? WHERE uid=2", math.MaxInt); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.transfer(1, 2, 1), ErrBalanceOverflow) {
		t.Fatal("overflow accepted")
	}
	balances(t, s, 100, math.MaxInt)
}
func TestTransferMissingRecipientAndRestart(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec("DELETE FROM storage WHERE uid=2"); err != nil {
		t.Fatal(err)
	}
	if err := s.transfer(1, 2, 20); err != nil {
		t.Fatal(err)
	}
	balances(t, s, 80, 20)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Open(time.Minute); err != nil {
		t.Fatal(err)
	}
	balances(t, s, 80, 20)
}
