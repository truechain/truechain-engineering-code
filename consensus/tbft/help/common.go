package help

import (
	"fmt"
	"bytes"
	"time"
	"encoding/json"
	"math/rand"
	"os"
	// "os/exec"
	// "os/signal"
	// "strings"
	"syscall"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/common"
)

type HexBytes []byte

// Fingerprint returns the first 6 bytes of a byte slice.
// If the slice is less than 6 bytes, the fingerprint
// contains trailing zeroes.
func Fingerprint(slice []byte) []byte {
	fingerprint := make([]byte, 6)
	copy(fingerprint, slice)
	return fingerprint
}

func IsZeros(slice []byte) bool {
	for _, byt := range slice {
		if byt != byte(0) {
			return false
		}
	}
	return true
}

func RightPadBytes(slice []byte, l int) []byte {
	if l < len(slice) {
		return slice
	}
	padded := make([]byte, l)
	copy(padded[0:len(slice)], slice)
	return padded
}

func LeftPadBytes(slice []byte, l int) []byte {
	if l < len(slice) {
		return slice
	}
	padded := make([]byte, l)
	copy(padded[l-len(slice):], slice)
	return padded
}

func TrimmedString(b []byte) string {
	trimSet := string([]byte{0})
	return string(bytes.TrimLeft(b, trimSet))
}

// PrefixEndBytes returns the end byteslice for a noninclusive range
// that would include all byte slices for which the input is the prefix
func PrefixEndBytes(prefix []byte) []byte {
	if prefix == nil {
		return nil
	}

	end := make([]byte, len(prefix))
	copy(end, prefix)
	finished := false

	for !finished {
		if end[len(end)-1] != byte(255) {
			end[len(end)-1]++
			finished = true
		} else {
			end = end[:len(end)-1]
			if len(end) == 0 {
				end = nil
				finished = true
			}
		}
	}
	return end
}
//-----------------------------------------------------------------------------
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MaxUint(a, b uint) uint {
	if a > b {
		return a
	}
	return b
}
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
//-----------------------------------------------------------------------------
const (
	strChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz" // 62 characters
)

func PanicSanity(v interface{}) {
	panic(fmt.Sprintf("Panicked on a Sanity Check: %v", v))
}

// Kill the running process by sending itself SIGTERM.
func Kill() error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
func RlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
func EnsureDir(dir string, mode os.FileMode) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, mode)
		if err != nil {
			return fmt.Errorf("Could not create directory %v. %v", dir, err)
		}
	}
	return nil
}
func RandStr(length int) string {
	// Str constructs a random alphanumeric string of given length.
	random := rand.New(rand.NewSource(time.Now().Unix()))
	chars := []byte{}
MAIN_LOOP:
	for {
		val := random.Int63()
		for i := 0; i < 10; i++ {
			v := int(val & 0x3f) // rightmost 6 bits
			if v >= 62 {         // only 62 characters in strChars
				val >>= 6
				continue
			} else {
				chars = append(chars, strChars[v])
				if len(chars) == length {
					break MAIN_LOOP
				}
				val >>= 6
			}
		}
	}	
return string(chars)
}
//-----------------------------------------------------------------------------

func MarshalBinaryBare(o interface{}) ([]byte,error) {
	return rlp.EncodeToBytes(o)
}
func MarshalJSON(o interface{}) ([]byte, error) {
	return json.Marshal(&o)
}
func UnmarshalBinaryBare(bz []byte, ptr interface{}) error {
	return rlp.DecodeBytes(bz, ptr)
}

//-----------------------------------------------------------------------------
func RandInt() int {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Int()
}
func RandIntn(n int) int {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Intn(n)
}
func Float64() float64  {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Float64()
}
func Int31n(n int32) int32 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Int31n(n)
}