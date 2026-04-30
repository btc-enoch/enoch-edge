package address

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// TestEnochRoundTrip exercises the path we hit on every wallet
// request: arbitrary pkh → enoch1 → back to pkh.
func TestEnochRoundTrip(t *testing.T) {
	pkh := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
	}
	addr, err := EncodeEnoch(pkh)
	if err != nil {
		t.Fatalf("EncodeEnoch: %v", err)
	}
	got, err := DecodeToPKH(addr)
	if err != nil {
		t.Fatalf("DecodeToPKH(%q): %v", addr, err)
	}
	if !bytes.Equal(got, pkh) {
		t.Errorf("round trip mismatch: got %x, want %x", got, pkh)
	}
}

// TestBitcoinMainnetP2WPKH uses the canonical BIP173 test vector
// to make sure we read native-segwit P2WPKH the same way bitcoind
// and every other wallet does.
func TestBitcoinMainnetP2WPKH(t *testing.T) {
	const addr = "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	expected, _ := hex.DecodeString("751e76e8199196d454941c45d1b3a323f1433bd6")

	got, err := DecodeToPKH(addr)
	if err != nil {
		t.Fatalf("DecodeToPKH: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Errorf("pkh = %x, want %x", got, expected)
	}
}

// TestRejectsTaproot keeps us honest about which witness versions
// are supported. Taproot uses bech32m, not bech32, so the underlying
// library rejects it before our witness-version check fires — the
// outcome (error returned) is what matters for callers.
func TestRejectsTaproot(t *testing.T) {
	const addr = "bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7k7grplx"
	if _, err := DecodeToPKH(addr); err == nil {
		t.Fatal("expected error for taproot address, got nil")
	}
}

// TestRejectsUnknownHRP makes sure an unrelated coin's bech32 with
// a valid checksum still fails — the HRP guard is doing real work.
func TestRejectsUnknownHRP(t *testing.T) {
	pkh := make([]byte, 20)
	conv, err := bech32.ConvertBits(pkh, 8, 5, true)
	if err != nil {
		t.Fatalf("ConvertBits: %v", err)
	}
	addr, err := bech32.Encode("doge", conv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := DecodeToPKH(addr); err == nil {
		t.Fatalf("expected error for hrp=doge, got nil")
	}
}

// TestRejectsGarbage covers the simplest input-validation path — a
// non-bech32 string should fail cleanly with a decode error rather
// than panic.
func TestRejectsGarbage(t *testing.T) {
	if _, err := DecodeToPKH("not-an-address"); err == nil {
		t.Fatal("expected error for garbage input, got nil")
	}
}
