package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// dailyTips each name a command a searcher may not have met, one line apiece.
// The list is short on purpose: a tip is worth showing once in a while, and a
// rotation through four reaches every one inside a working week (#4629).
var dailyTips = []string{
	"tip: `deja blame <file>:<line>` names the session that wrote that line",
	"tip: `deja fix \"<error text>\"` shows what was run after that error before",
	"tip: `deja how <what>` lists the commands this project actually ran for it",
	"tip: `deja view` opens your sessions, recalls and notes as one local page",
}

// tipInterval is how long one tip stands before the next may be shown.
const tipInterval = 24 * time.Hour

// maybeTip writes at most one tip per tipInterval to w, the next in rotation.
// stamp holds "<unix time shown> <index of the next tip>". The stamp is
// written before the tip: one that cannot be recorded would otherwise show on
// every search, which is the opposite of once a day, so it is not shown.
func maybeTip(w io.Writer, stamp string, now time.Time) {
	next := 0
	if b, err := os.ReadFile(stamp); err == nil {
		parts := strings.Fields(string(b))
		if len(parts) == 2 {
			if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil && now.Sub(time.Unix(ts, 0)) < tipInterval {
				return
			}
			if i, err := strconv.Atoi(parts[1]); err == nil && i >= 0 && i < len(dailyTips) {
				next = i
			}
		}
	}
	record := fmt.Sprintf("%d %d", now.Unix(), (next+1)%len(dailyTips))
	if err := os.WriteFile(stamp, []byte(record), 0o600); err != nil {
		return
	}
	fmt.Fprintln(w, dailyTips[next])
}
