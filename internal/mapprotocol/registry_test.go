package mapprotocol

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"

	goeth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/compass/pkg/msg"
)

// fakeCaller answers contract reads from a canned output, so the light manager can
// be exercised without an RPC endpoint.
type fakeCaller struct {
	output []byte
	err    error

	mu    sync.Mutex
	calls int
	to    common.Address
}

func (f *fakeCaller) CallContract(_ context.Context, call goeth.CallMsg, _ *big.Int) ([]byte, error) {
	f.mu.Lock()
	f.calls++
	if call.To != nil {
		f.to = *call.To
	}
	f.mu.Unlock()
	return f.output, f.err
}

func TestRegistryRoundTrip(t *testing.T) {
	r := NewRegistry()

	r.SetMapID("212")
	if got := r.MapID(); got != "212" {
		t.Fatalf("MapID = %q, want 212", got)
	}

	r.SetChainName(97, "bsc")
	if name, ok := r.ChainName(97); !ok || name != "bsc" {
		t.Fatalf("ChainName(97) = %q, %v; want bsc, true", name, ok)
	}
	if got := r.ChainNameOf(999); got != "" {
		t.Fatalf("ChainNameOf(unknown) = %q, want empty", got)
	}

	r.SetMosAddress(97, "0xabc")
	if addr, ok := r.MosAddress(97); !ok || addr != "0xabc" {
		t.Fatalf("MosAddress(97) = %q, %v; want 0xabc, true", addr, ok)
	}

	if _, ok := r.MosCall(97); ok {
		t.Fatal("MosCall(97) reported a caller that was never registered")
	}
}

func TestRegistryMap2OtherSyncedHeight(t *testing.T) {
	r := NewRegistry()

	// The old code indexed a map and called the result, so an unregistered chain
	// panicked. It must be an error instead.
	if _, err := r.Map2OtherSyncedHeight(97); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Map2OtherSyncedHeight on unregistered chain err = %v, want ErrNotRegistered", err)
	}

	r.SetMap2OtherHeight(97, func() (*big.Int, error) { return big.NewInt(1234), nil })
	height, err := r.Map2OtherSyncedHeight(97)
	if err != nil {
		t.Fatalf("Map2OtherSyncedHeight: %v", err)
	}
	if height.Int64() != 1234 {
		t.Fatalf("height = %d, want 1234", height.Int64())
	}
}

func TestRegistryMap2OtherInitHeightsIsSnapshot(t *testing.T) {
	r := NewRegistry()
	r.SetMap2OtherInitHeight(97, big.NewInt(10))

	snapshot := r.Map2OtherInitHeights()
	r.SetMap2OtherInitHeight(1, big.NewInt(20))

	if len(snapshot) != 1 {
		t.Fatalf("snapshot len = %d, want 1: callers must iterate a copy", len(snapshot))
	}
}

func TestRegistryWithoutLightManager(t *testing.T) {
	r := NewRegistry()

	// Before this change these reads went through function variables that returned
	// (nil, nil) until startup installed the real ones, so the caller dereferenced
	// a nil height.
	if _, err := r.Chain2MapHeight(97); err == nil {
		t.Fatal("Chain2MapHeight without a light manager returned no error")
	}
	if _, err := r.LightNodeType(MethodOfNodeType, big.NewInt(97)); err == nil {
		t.Fatal("LightNodeType without a light manager returned no error")
	}
	if _, _, err := r.Eth2MapNumber(97); err == nil {
		t.Fatal("Eth2MapNumber without a light manager returned no error")
	}
}

func TestRegistryChain2MapHeightUsesLightManager(t *testing.T) {
	packed, err := Height.Methods[MethodOfHeaderHeight].Outputs.Pack(big.NewInt(4242))
	if err != nil {
		t.Fatalf("pack headerHeight output: %v", err)
	}

	addr := common.HexToAddress("0x000068656164657273746F726541646472657373")
	caller := &fakeCaller{output: packed}

	r := NewRegistry()
	r.SetLightManager(NewLightManager(caller, addr))

	height, err := r.Chain2MapHeight(97)
	if err != nil {
		t.Fatalf("Chain2MapHeight: %v", err)
	}
	if height.Int64() != 4242 {
		t.Fatalf("height = %d, want 4242", height.Int64())
	}
	if caller.to != addr {
		t.Fatalf("called %s, want the light manager address %s", caller.to, addr)
	}
}

// TestRegistryConcurrentAccess is the reason this type exists: the expose proof API
// registers chains from one goroutine per HTTP request while the sync loops read
// the same state. Run with -race.
func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	const workers = 16

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		id := msg.ChainId(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				r.SetMap2OtherInitHeight(id, big.NewInt(int64(n)))
				r.SetMap2OtherHeight(id, func() (*big.Int, error) { return big.NewInt(int64(n)), nil })
				r.SetChainName(id, "chain")
				r.SetMosAddress(id, "0xabc")
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				r.Map2OtherInitHeights()
				_, _ = r.Map2OtherInitHeight(id)
				_, _ = r.Map2OtherHeight(id)
				_ = r.ChainNameOf(id)
				_, _ = r.MosCall(id)
				_, _ = r.SignerCall(id)
				_, _ = r.LightNodeCall(id)
			}
		}()
	}
	wg.Wait()
}
