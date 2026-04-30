// Package address normalizes user-facing Bitcoin / Enoch addresses
// into the 20-byte pubkey-hash that the operator uses as its lookup
// key. Lives at the edge — the operator only speaks Enoch-native
// `enoch1...`, so multi-format support is a wallet UX concern, not
// a protocol concern.
package address

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// EnochHRP is the human-readable prefix for Enoch L2 bech32 addresses.
// Same payload form as BCH/Zcash's bech32-without-segwit: HRP + 20-byte
// hash + checksum, no witness-version byte.
const EnochHRP = "enoch"

// DecodeToPKH accepts any of:
//   - enoch1...                       (Enoch L2 bech32, no witness version)
//   - bc1q... / tb1q... / bcrt1q...   (Bitcoin native segwit P2WPKH, v0)
//
// And returns the 20-byte pubkey-hash that both forms encode. Taproot
// (segwit v1, bech32m) is rejected — it uses a 32-byte x-only key,
// not a hash160, so there's no shared pkh to look up.
func DecodeToPKH(addr string) ([]byte, error) {
	hrp, data, err := bech32.Decode(addr)
	if err != nil {
		return nil, fmt.Errorf("bech32 decode: %w", err)
	}
	switch hrp {
	case EnochHRP:
		// Enoch L2 — no witness version, full payload is the pkh.
		pkh, err := bech32.ConvertBits(data, 5, 8, false)
		if err != nil {
			return nil, fmt.Errorf("bech32 bits: %w", err)
		}
		if len(pkh) != 20 {
			return nil, fmt.Errorf("expected 20-byte payload, got %d", len(pkh))
		}
		return pkh, nil
	case "bc", "tb", "bcrt":
		// Bitcoin segwit. data[0] is the witness version, data[1:] is
		// the program in 5-bit groups.
		if len(data) < 1 {
			return nil, fmt.Errorf("segwit data too short")
		}
		witver := data[0]
		if witver != 0 {
			return nil, fmt.Errorf("only segwit v0 (P2WPKH) supported, got v%d", witver)
		}
		program, err := bech32.ConvertBits(data[1:], 5, 8, false)
		if err != nil {
			return nil, fmt.Errorf("segwit program bits: %w", err)
		}
		if len(program) != 20 {
			return nil, fmt.Errorf("P2WPKH program must be 20 bytes, got %d", len(program))
		}
		return program, nil
	default:
		return nil, fmt.Errorf("unsupported address HRP: %q", hrp)
	}
}

// EncodeEnoch is the inverse direction edge needs when proxying to
// the operator: given a pkh (decoded from any input form), produce
// the enoch1... string the operator's lookup endpoints expect.
func EncodeEnoch(pkh []byte) (string, error) {
	if len(pkh) != 20 {
		return "", fmt.Errorf("pkh must be 20 bytes, got %d", len(pkh))
	}
	conv, err := bech32.ConvertBits(pkh, 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode(EnochHRP, conv)
}
