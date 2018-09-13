package help

import (
	"fmt"
	"bytes"
	"time"
	"encoding/json"
	"encoding/binary"
	"math/rand"
	"strings"
	"os"
	"io"
	// "os/exec"
	// "os/signal"
	// "strings"
	"syscall"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/common"
)

type HexBytes []byte
type Address = HexBytes

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
func EqualHashes(hash1,hash2 []byte) bool {
	if len(hash1) == 0 || len(hash2) == 0{
		return false
	}
	return bytes.Equal(hash1, hash2)
}
// ProtocolAndAddress splits an address into the protocol and address components.
// For instance, "tcp://127.0.0.1:8080" will be split into "tcp" and "127.0.0.1:8080".
// If the address has no protocol prefix, the default is "tcp".
func ProtocolAndAddress(listenAddr string) (string, string) {
	protocol, address := "tcp", listenAddr
	parts := strings.SplitN(address, "://", 2)
	if len(parts) == 2 {
		protocol, address = parts[0], parts[1]
	}
	return protocol, address
}
//-----------------------------------------------------------------------------
// Panics if error.
func MustMarshalBinaryBare(o interface{}) []byte {
	bz, err := MarshalBinaryBare(o)
	if err != nil {
		panic(err)
	}
	return bz
}
func MarshalBinaryBare(o interface{}) ([]byte,error) {
	return rlp.EncodeToBytes(o)
}
func MarshalJSON(o interface{}) ([]byte, error) {
	return json.Marshal(&o)
}
func UnmarshalBinaryBare(bz []byte, ptr interface{}) error {
	return rlp.DecodeBytes(bz, ptr)
}
// MarshalBinaryWriter writes the bytes as would be returned from
// MarshalBinary to the writer w.
func MarshalBinaryWriter(w io.Writer, o interface{}) (n int64, err error) {
	var bz, _n = []byte(nil), int(0)
	bz, err = MarshalBinaryBare(o)
	if err != nil {
		return 0, err
	}
	_n, err = w.Write(bz) // TODO: handle overflow in 32-bit systems.
	n = int64(_n)
	return
}
// Like UnmarshalBinaryBare, but will first read the byte-length prefix.
// UnmarshalBinaryReader will panic if ptr is a nil-pointer.
// If maxSize is 0, there is no limit (not recommended).
func UnmarshalBinaryReader(r io.Reader, ptr interface{}, maxSize int64) (n int64, err error) {
	if maxSize < 0 {
		panic("maxSize cannot be negative.")
	}

	// Read byte-length prefix.
	var l int64
	var buf [binary.MaxVarintLen64]byte
	for i := 0; i < len(buf); i++ {
		_, err = r.Read(buf[i : i+1])
		if err != nil {
			return
		}
		n += 1
		if buf[i]&0x80 == 0 {
			break
		}
		if n >= maxSize {
			err = fmt.Errorf("Read overflow, maxSize is %v but uvarint(length-prefix) is itself greater than maxSize.", maxSize)
		}
	}
	u64, _ := binary.Uvarint(buf[:])
	if err != nil {
		return
	}
	if maxSize > 0 {
		if uint64(maxSize) < u64 {
			err = fmt.Errorf("Read overflow, maxSize is %v but this amino binary object is %v bytes.", maxSize, u64)
			return
		}
		if (maxSize - n) < int64(u64) {
			err = fmt.Errorf("Read overflow, maxSize is %v but this length-prefixed amino binary object is %v+%v bytes.", maxSize, n, u64)
			return
		}
	}
	l = int64(u64)
	if l < 0 {
		err = fmt.Errorf("Read overflow, this implementation can't read this because, why would anyone have this much data? Hello from 2018.")
	}

	// Read that many bytes.
	var bz = make([]byte, l, l)
	_, err = io.ReadFull(r, bz)
	if err != nil {
		return
	}
	n += l

	// Decode.
	err = UnmarshalBinaryBare(bz, ptr)
	return
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
func RandFloat64() float64  {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Float64()
}
func RandInt31n(n int32) int32 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return random.Int31n(n)
}
func RandPerm(n int) []int {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	perm := random.Perm(n)
	return perm
}
func RandInt63n(n int64) int64 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	i63n := random.Int63n(n)
	return i63n
}