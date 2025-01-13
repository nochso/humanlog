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
	log.DebugLevel: color.New(color.FgBlue),
	log.InfoLevel:  color.New(color.FgGreen),
	log.WarnLevel:  color.New(color.FgYellow),
	log.ErrorLevel: color.New(color.FgRed),
	log.FatalLevel: color.New(color.BgRed, color.FgWhite),
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
	lengths   map[string]keyStat
	buf       *bytes.Buffer
}

type keyStat struct {
	MaxLength      int
	Count          int
	RightAlignable int
}

// New return a new [Handler] writing to [w].
func New(w io.Writer) *Handler {
	h := &Handler{
		lengths:   map[string]keyStat{},
		Writer:    w,
		buf:       &bytes.Buffer{},
		Timestamp: "15:04:05.000",
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
	names := h.sortNames(e, e.Fields.Names())

	h.mu.Lock()
	defer h.mu.Unlock()

	h.buf.Reset()
	_, err := colr.Fprintf(h.buf, "%s %s", e.Timestamp.Format(h.Timestamp), level)
	if err != nil {
		return err
	}
	if e.Message != "" {
		_, err = fmt.Fprintf(h.buf, " %s", e.Message)
		if err != nil {
			return err
		}
	}
	for i, name := range names {
		if name == "source" {
			continue
		}
		err = h.writeNameValue(e, name, i, names)
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(h.buf)
	if err != nil {
		return err
	}
	_, err = h.buf.WriteTo(h.Writer)
	return err
}

func (h *Handler) writeNameValue(e *log.Entry, name string, i int, names []string) error {
	val := e.Fields.Get(name)
	if dur, ok := val.(time.Duration); ok {
		val = Duration(dur)
	}
	sw := runewidth.StringWidth(fmt.Sprintf("%v", val))
	kstat, _ := h.lengths[name]
	kstat.MaxLength = max(kstat.MaxLength, sw)
	kstat.Count++
	if sw+20 < kstat.MaxLength {
		kstat.MaxLength = sw
	}
	isRight := h.isTypeRightAlignable(val)
	if isRight {
		kstat.RightAlignable++
	}
	isRight = kstat.RightAlignable > kstat.Count/2
	h.lengths[name] = kstat
	if isRight {
		_, err := fmt.Fprintf(h.buf, " %s=%*v", h.getKeyColor(name).Sprint(name), kstat.MaxLength, val)
		if err != nil {
			return err
		}
		return nil
	}
	var pad string
	if sw < kstat.MaxLength && i+1 != len(names) {
		pad = strings.Repeat(" ", kstat.MaxLength-sw)
	}
	_, err := fmt.Fprintf(h.buf, " %s=%v%s", h.getKeyColor(name).Sprint(name), val, pad)
	return err
}

func (h *Handler) sortNames(e *log.Entry, names []string) []string {
	sort.Slice(names, func(a, b int) bool {
		aright := h.isTypeRightAlignable(e.Fields.Get(names[a]))
		bright := h.isTypeRightAlignable(e.Fields.Get(names[b]))
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

func (h *Handler) isTypeRightAlignable(x interface{}) bool {
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
	r, g, b := mapColor(sum>>24), mapColor(sum>>16), mapColor(sum>>8)
	return color.RGB(int(r), int(g), int(b))
}

func mapColor(v uint32) byte {
	v = uint32(byte(v))
	return byte(v*5/10 + 105)
}

var durations = []struct {
	d time.Duration
	s string
}{
	{d: time.Hour * 24 * 365, s: "%dy"},
	{d: time.Hour * 24 * 30, s: "%dm"},
	{d: time.Hour * 24 * 7, s: "%dw"},
	{d: time.Hour * 24, s: "%dd"},
	{d: time.Hour, s: "%dh"},
	{d: time.Minute, s: "%dm"},
	{d: time.Second, s: "%ds"},
}

func Duration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d)
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.4gµs", float64(d)/1000)
	} else if d < time.Second {
		return fmt.Sprintf("%.4gms", float64(d)/1000/1000)
	} else if d < time.Minute {
		return fmt.Sprintf("%.4gs", float64(d)/1000/1000/1000)
	}
	var s string
	last := -1
	for i := range durations {
		if d >= durations[i].d {
			if i-1 != last { // stop instead of skipping a unit
				break
			}
			last = i
			s += fmt.Sprintf(durations[i].s, d/durations[i].d)
			d %= durations[i].d
			if len(s) >= 5 {
				break
			}
		}
	}
	return s
}
