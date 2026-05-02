// Package address normalizes user-facing Bitcoin / Enoch addresses
// into the canonical Enoch form the operator expects. Lives at the
// edge — multi-format support is a wallet UX concern, not a protocol
// concern.
package address

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// EnochHRP is the human-readable prefix for Enoch L2 bech32 addresses.
// Two payload shapes coexist:
//   - legacy P2PKH: BIP-173 bech32 + 20-byte HASH160(pubkey), no
//     witness-version byte. `enoch1...`
//   - P2TR (post-#109): BIP-350 bech32m + witness version 1 + 32-byte
//     x-only Taproot output key. `enoch1p...`
const EnochHRP = "enoch"

// NormalizeToEnoch accepts any user-facing wallet address and returns
// the canonical Enoch form the operator's HTTP API expects. Used by
// the wallet-facing edge handlers (balance, utxos, address_history,
// events SSE filter) so the iOS / web wallet can pass through
// whatever address the user typed.
//
// Accepted inputs:
//   - enoch1...   (legacy P2PKH)        → returned unchanged after validation
//   - enoch1p...  (post-#109 P2TR)      → returned unchanged after validation
//   - bc1q.../tb1q.../bcrt1q...         (Bitcoin segwit v0 P2WPKH)
//       → decoded to 20-byte pkh, re-encoded as enoch1...
//
// Anything else (Bitcoin Taproot, segwit v1+ on bc/tb/bcrt, P2SH,
// other HRPs) is rejected — the wallet has no L2 lookup for them.
func NormalizeToEnoch(addr string) (string, error) {
	hrp, data, _, err := bech32.DecodeGeneric(addr)
	if err != nil {
		return "", fmt.Errorf("bech32 decode: %w", err)
	}
	switch hrp {
	case EnochHRP:
		// Either form — pass through. The operator's
		// DecodeEnochAddressAny dispatches on bech32 vs bech32m
		// internally, so we don't need to re-encode here.
		return addr, nil
	case "bc", "tb", "bcrt":
		// Bitcoin segwit. data[0] is witness version, data[1:] is the
		// program in 5-bit groups. Only v0 (P2WPKH) maps to a 20-byte
		// pkh that has an Enoch P2PKH equivalent.
		if len(data) < 1 {
			return "", fmt.Errorf("segwit data too short")
		}
		if data[0] != 0 {
			return "", fmt.Errorf("only segwit v0 (P2WPKH) supported on Bitcoin HRPs, got v%d", data[0])
		}
		program, err := bech32.ConvertBits(data[1:], 5, 8, false)
		if err != nil {
			return "", fmt.Errorf("segwit program bits: %w", err)
		}
		if len(program) != 20 {
			return "", fmt.Errorf("P2WPKH program must be 20 bytes, got %d", len(program))
		}
		conv, err := bech32.ConvertBits(program, 8, 5, true)
		if err != nil {
			return "", err
		}
		return bech32.Encode(EnochHRP, conv)
	default:
		return "", fmt.Errorf("unsupported address HRP: %q", hrp)
	}
}

