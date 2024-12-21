// Package humanlog implements a colored text handler suitable for command-line interfaces.
package humanlog

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/fatih/color"
	colorable "github.com/mattn/go-colorable"
	"github.com/mattn/go-runewidth"
)

// Default handler outputs to stderr.
var Default = New(os.Stderr)

// LevelColors maps log levels to a color.
var LevelColors = [...]*color.Color{
	log.DebugLevel: color.New(color.FgWhite),
	log.InfoLevel:  color.New(color.FgBlue),
	log.WarnLevel:  color.New(color.FgYellow),
	log.ErrorLevel: color.New(color.FgRed),
	log.FatalLevel: color.New(color.FgHiRed),
}

var KeyColors = [...]*color.Color{
	color.New(color.FgRed),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgMagenta),
	color.New(color.FgCyan),
	color.New(color.FgHiRed),
	color.New(color.FgHiGreen),
	color.New(color.FgHiYellow),
	color.New(color.FgHiBlue),
	color.New(color.FgHiMagenta),
	color.New(color.FgHiCyan),
}

// LevelSymbol maps log levels to a string representing it.
var LevelSymbol = [...]string{
	log.DebugLevel: "D",
	log.InfoLevel:  "I",
	log.WarnLevel:  "W",
	log.ErrorLevel: "E",
	log.FatalLevel: "F",
}

// Handler implements [github.com/apex/log.Handler]
type Handler struct {
	mu        sync.Mutex
	Writer    io.Writer
	Timestamp string
	lengths   map[string]int
	buf       *bytes.Buffer
}

// New return a new [Handler] writing to [w].
func New(w io.Writer) *Handler {
	h := &Handler{
		lengths: map[string]int{},
		Writer:  w,
		buf:     &bytes.Buffer{},
	}
	if f, ok := w.(*os.File); ok {
		h.Writer = colorable.NewColorable(f)
	}
	return h
}

// HandleLog implements [github.com/apex/log.Handler].
func (h *Handler) HandleLog(e *log.Entry) error {
	colr := LevelColors[e.Level]
	level := LevelSymbol[e.Level]
	names := sortNames(e, e.Fields.Names())
	sw := runewidth.StringWidth(e.Message)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.lengths["msg"] = max(h.lengths["msg"], sw)
	colr.Fprintf(h.buf, "%s %s", e.Timestamp.Format(h.Timestamp), level)
	fmt.Fprintf(h.buf, " %*s", -h.lengths["msg"], e.Message)
	for i, name := range names {
		if name == "source" {
			continue
		}
		val := e.Fields.Get(name)
		sw := runewidth.StringWidth(fmt.Sprintf("%v", val))
		h.lengths[name] = max(h.lengths[name], sw)
		l := h.lengths[name]
		if sw+20 < l {
			l = sw
			h.lengths[name] = sw
		}
		if isTypeRightAlignable(val) {
			fmt.Fprintf(h.buf, " %s=%*v", h.getKeyColor(name).Sprint(name), h.lengths[name], val)
		} else {
			var pad string
			if sw < l && i+1 != len(names) {
				pad = strings.Repeat(" ", l-sw)
			}
			fmt.Fprintf(h.buf, " %s=%v%s", h.getKeyColor(name).Sprint(name), val, pad)
		}
	}
	fmt.Fprintln(h.buf)
	h.buf.WriteTo(h.Writer)
	h.buf.Reset()
	return nil
}

func sortNames(e *log.Entry, names []string) []string {
	sort.Slice(names, func(a, b int) bool {
		aright := isTypeRightAlignable(e.Fields.Get(names[a]))
		bright := isTypeRightAlignable(e.Fields.Get(names[b]))
		if aright != bright {
			return aright
		}
		return strings.Compare(names[a], names[b]) == -1
	})
	return names
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var reNumType = regexp.MustCompile(`(?i)^\d+(\.\d+)? ?([a-z]{1,5})?$`)

func isTypeRightAlignable(x interface{}) bool {
	switch v := x.(type) {
	case uint, uint8, uint16, uint32, uint64,
		int, int8, int16, int32, int64,
		float32, float64,
		complex64, complex128,
		time.Duration, *time.Duration:
		return true
	case string:
		return reNumType.MatchString(v)
	case []byte:
		return reNumType.Match(v)
	default:
		return false
	}
}

func (h *Handler) getKeyColor(key string) *color.Color {
	if key == "error" {
		return KeyColors[6]
	}
	sum := crc32.ChecksumIEEE([]byte(key))
	sum = sum % uint32(len(KeyColors))
	return KeyColors[sum]
}
