package types

import "time"

// Millis is a Unix timestamp in milliseconds, the unit Bitfinex
// uses on the wire (MTS fields). The distinct type makes unit
// mix-ups a compile error instead of a silent bug.
type Millis int64

// MillisFromTime converts a time.Time to milliseconds.
func MillisFromTime(t time.Time) Millis { return Millis(t.UnixMilli()) }

// MillisNow returns the current wall-clock time in milliseconds.
func MillisNow() Millis { return Millis(time.Now().UnixMilli()) }

// Time converts the timestamp to a UTC time.Time.
func (m Millis) Time() time.Time { return time.UnixMilli(int64(m)).UTC() }

// Int64 returns the raw milliseconds value.
func (m Millis) Int64() int64 { return int64(m) }
