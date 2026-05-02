package address

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// TestNormalizeLegacyEnochPassthrough — a valid enoch1... legacy
// (P2PKH) address comes back unchanged. The operator's
// DecodeEnochAddressAny owns the full validation; edge just confirms
// the bech32 envelope decodes cleanly so a typo doesn't open a stream.
func TestNormalizeLegacyEnochPassthrough(t *testing.T) {
	const addr = "enoch1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqljsyzs" // 20 zero bytes
	got, err := NormalizeToEnoch(addr)
	if err != nil {
		t.Fatalf("NormalizeToEnoch(%q): %v", addr, err)
	}
	if got != addr {
		t.Errorf("legacy enoch1 should pass through; got %q want %q", got, addr)
	}
}

// TestNormalizeP2TREnochPassthrough — post-#109 enoch1p... addresses
// must pass through. Pre-#109 this was rejected ("expected 20-byte
// payload, got 33") which broke the iOS wallet's SSE filter.
func TestNormalizeP2TREnochPassthrough(t *testing.T) {
	const addr = "enoch1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxq65mkkv"
	got, err := NormalizeToEnoch(addr)
	if err != nil {
		t.Fatalf("NormalizeToEnoch(%q): %v", addr, err)
	}
	if got != addr {
		t.Errorf("P2TR enoch1p should pass through; got %q want %q", got, addr)
	}
}

// TestNormalizeBitcoinP2WPKH — the canonical BIP-173 mainnet vector
// should re-encode as the corresponding enoch1... form. This is the
// "wallet typed bc1q..." case, where edge bridges the user's input
// form into the operator's enoch1 namespace.
func TestNormalizeBitcoinP2WPKH(t *testing.T) {
	const btcAddr = "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	got, err := NormalizeToEnoch(btcAddr)
	if err != nil {
		t.Fatalf("NormalizeToEnoch(%q): %v", btcAddr, err)
	}
	// Same 20-byte program, encoded under the enoch HRP. We compute the
	// expected string from the program directly to keep the test
	// honest if the bech32 lib changes.
	const expectedPKHHex = "751e76e8199196d454941c45d1b3a323f1433bd6"
	pkh := mustHex(t, expectedPKHHex)
	conv, err := bech32.ConvertBits(pkh, 8, 5, true)
	if err != nil {
		t.Fatalf("ConvertBits: %v", err)
	}
	want, err := bech32.Encode(EnochHRP, conv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got != want {
		t.Errorf("P2WPKH translation mismatch: got %q want %q", got, want)
	}
}

// TestNormalizeRejectsBitcoinTaproot — bc1p... is *Bitcoin* Taproot
// (output key), which has no Enoch L2 mapping. Should fail cleanly.
func TestNormalizeRejectsBitcoinTaproot(t *testing.T) {
	const addr = "bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr"
	if _, err := NormalizeToEnoch(addr); err == nil {
		t.Fatal("expected error for Bitcoin Taproot address, got nil")
	}
}

// TestNormalizeRejectsUnknownHRP — bech32 with a valid checksum but
// an unrelated HRP must fail. Guards against accidentally treating
// a doge1... address as a Bitcoin one.
func TestNormalizeRejectsUnknownHRP(t *testing.T) {
	pkh := make([]byte, 20)
	conv, err := bech32.ConvertBits(pkh, 8, 5, true)
	if err != nil {
		t.Fatalf("ConvertBits: %v", err)
	}
	addr, err := bech32.Encode("doge", conv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := NormalizeToEnoch(addr); err == nil {
		t.Fatalf("expected error for hrp=doge, got nil")
	}
}

// TestNormalizeRejectsGarbage — a non-bech32 string fails cleanly
// rather than panicking deep inside the decoder.
func TestNormalizeRejectsGarbage(t *testing.T) {
	if _, err := NormalizeToEnoch("not-an-address"); err == nil {
		t.Fatal("expected error for garbage input, got nil")
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var hi, lo byte
		switch {
		case s[i] >= '0' && s[i] <= '9':
			hi = s[i] - '0'
		case s[i] >= 'a' && s[i] <= 'f':
			hi = s[i] - 'a' + 10
		default:
			t.Fatalf("bad hex char %q", s[i])
		}
		switch {
		case s[i+1] >= '0' && s[i+1] <= '9':
			lo = s[i+1] - '0'
		case s[i+1] >= 'a' && s[i+1] <= 'f':
			lo = s[i+1] - 'a' + 10
		default:
			t.Fatalf("bad hex char %q", s[i+1])
		}
		out[i/2] = hi<<4 | lo
	}
	return out
}
