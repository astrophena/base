// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package progressbar implements a simple, non-animated progress bar, inspired by
// [alive-progress].
//
// [alive-progress]: https://github.com/rsalmei/alive-progress
package progressbar

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Bar is a simple, non-animated progress bar.
type Bar struct {
	mu          sync.Mutex
	w           io.Writer
	total       int
	current     int
	titleText   string
	startTime   time.Time
	barWidth    int
	aborted     bool
	failed      bool
	interactive bool
	stopMonitor chan struct{}
	monitorDone chan struct{}
}

// New creates a new [Bar].
func New(w io.Writer, total int, interactive bool) *Bar {
	return &Bar{
		w:           w,
		total:       total,
		barWidth:    40,
		interactive: interactive,
		stopMonitor: make(chan struct{}),
		monitorDone: make(chan struct{}),
	}
}

// Start begins the progress bar timer and rendering.
func (pb *Bar) Start() {
	pb.startTime = time.Now()
	if pb.interactive {
		go pb.monitor()
	}
	pb.render(false)
}

// monitor updates the timer in the background.
func (pb *Bar) monitor() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-pb.stopMonitor:
			close(pb.monitorDone)
			return
		case <-ticker.C:
			pb.render(false)
		}
	}
}

// Stop finalizes the progress bar.
func (pb *Bar) Stop(failed bool) {
	pb.mu.Lock()
	pb.failed = failed
	if !failed && pb.current < pb.total {
		pb.aborted = true
	}
	pb.mu.Unlock()

	if pb.interactive {
		close(pb.stopMonitor)
		<-pb.monitorDone
	}

	pb.render(true)
}

// Increment advances the progress bar by one.
func (pb *Bar) Increment() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.current++
	pb.renderContent(false)
}

// SetTitle updates the title text shown next to the bar.
func (pb *Bar) SetTitle(text string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.titleText = text
	pb.renderContent(false)
}

// Printf prints text without breaking the progress bar rendering.
func (pb *Bar) Printf(format string, args ...any) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.interactive {
		fmt.Fprint(pb.w, "\r\033[K")
	}
	fmt.Fprintf(pb.w, format+"\n", args...)
	pb.renderContent(false)
}

func (pb *Bar) render(final bool) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.renderContent(final)
}

func (pb *Bar) formatTime(d time.Duration) string {
	s := d.Seconds()
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	m := int(s) / 60
	sRem := int(s) % 60
	return fmt.Sprintf("%dm%ds", m, sRem)
}

func (pb *Bar) renderContent(final bool) {
	if !pb.interactive {
		return
	}

	elapsed := time.Since(pb.startTime)
	percent := 1.0
	if pb.total > 0 {
		percent = float64(pb.current) / float64(pb.total)
	}

	barFillChar := "█"
	barEmptyChar := " "
	endChar := "|"

	warningFlag := false
	if pb.failed {
		endChar = "✗︎"
		warningFlag = true
	} else if pb.aborted {
		endChar = "⚠︎"
		warningFlag = true
	}

	if !final {
		endChar = "|"
	}

	filledLen := min(int(float64(pb.barWidth)*percent), pb.barWidth)

	barStr := strings.Repeat(barFillChar, filledLen)

	var barVisual string
	if warningFlag && final {
		endCharRunes := []rune(endChar)
		endCharLen := len(endCharRunes)
		if filledLen+endCharLen <= pb.barWidth {
			newPadLen := pb.barWidth - filledLen - endCharLen
			barVisual = fmt.Sprintf("|%s%s%s|", barStr, endChar, strings.Repeat(barEmptyChar, newPadLen))
		} else {
			keepLen := pb.barWidth - endCharLen
			barVisual = fmt.Sprintf("|%s%s|", string([]rune(barStr)[:keepLen]), endChar)
		}
	} else {
		padding := strings.Repeat(barEmptyChar, pb.barWidth-len([]rune(barStr)))
		barVisual = fmt.Sprintf("|%s%s|", barStr, padding)
	}

	prefix := " "
	if warningFlag {
		prefix = " (!) "
	}
	dispPercent := int(percent * 100)
	stats := fmt.Sprintf("%s%d/%d [%d%%] in %s", prefix, pb.current, pb.total, dispPercent, pb.formatTime(elapsed))

	line := fmt.Sprintf("%s%s", barVisual, stats)

	if pb.titleText != "" {
		line = fmt.Sprintf("%s %s", pb.titleText, line)
	}

	fmt.Fprintf(pb.w, "\r%s\033[K", line)
	if final {
		fmt.Fprintln(pb.w)
	}
}
