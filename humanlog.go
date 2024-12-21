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
		lengths:   map[string]int{},
		Writer:    w,
		buf:       &bytes.Buffer{},
		Timestamp: "060102 15:04:05",
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
		h.writeNameValue(e, name, i, names)
	}
	fmt.Fprintln(h.buf)
	h.buf.WriteTo(h.Writer)
	h.buf.Reset()
	return nil
}

func (h *Handler) writeNameValue(e *log.Entry, name string, i int, names []string) {
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
		return
	}
	var pad string
	if sw < l && i+1 != len(names) {
		pad = strings.Repeat(" ", l-sw)
	}
	fmt.Fprintf(h.buf, " %s=%v%s", h.getKeyColor(name).Sprint(name), val, pad)
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
		return LevelColors[log.ErrorLevel]
	}
	sum := crc32.ChecksumIEEE([]byte(key))
	r, g, b := mapColor(byte(sum>>24)), mapColor(byte(sum>>16)), mapColor(byte(sum>>8))
	return color.RGB(int(r), int(g), int(b))
}

func mapColor(v byte) byte {
	return v/2 + 64
}
