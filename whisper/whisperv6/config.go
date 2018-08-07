
package whisperv6

// Config represents the configuration state of a whisper node.
type Config struct {
	MaxMessageSize     uint32  `toml:",omitempty"`
	MinimumAcceptedPOW float64 `toml:",omitempty"`
}

// DefaultConfig represents (shocker!) the default configuration.
var DefaultConfig = Config{
	MaxMessageSize:     DefaultMaxMessageSize,
	MinimumAcceptedPOW: DefaultMinimumPoW,
}
