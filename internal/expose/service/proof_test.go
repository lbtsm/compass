package service

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestFindLogByIndexReturnsExactLog(t *testing.T) {
	logs := []types.Log{{Index: 3}, {Index: 7}}
	got, err := findLogByIndex(logs, 100, 7)
	if err != nil {
		t.Fatalf("findLogByIndex returned error: %v", err)
	}
	if got.Index != 7 {
		t.Fatalf("findLogByIndex returned index %d, want 7", got.Index)
	}
}

func TestFindLogByIndexRejectsMissingLog(t *testing.T) {
	_, err := findLogByIndex([]types.Log{{Index: 3}}, 100, 7)
	if err == nil {
		t.Fatal("findLogByIndex returned nil error for a missing log")
	}
	if !strings.Contains(err.Error(), "block(100)") || !strings.Contains(err.Error(), "logIndex(7)") {
		t.Fatalf("error lacks lookup context: %v", err)
	}
}
