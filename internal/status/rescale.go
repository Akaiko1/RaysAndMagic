package status

import (
	"reflect"

	"ugataima/internal/config"
	"ugataima/internal/mathutil"
)

// TickScaler is a persisted span counted in frames (DualClock,
// items.RoundClock): a save read at another tick rate rescales it.
type TickScaler interface{ ScaleTicks(k float64) }

var tickScalerType = reflect.TypeFor[TickScaler]()

// ScaleTicks keeps the clock's seconds at a tick rate k times the old one:
// frames and the frames-per-turn rate scale, turns stay.
func (c *DualClock) ScaleTicks(k float64) {
	c.Frames, c.Rate = mathutil.ScaleCount(c.Frames, k), mathutil.ScaleCount(c.Rate, k)
}

// RescaleSaved brings the frame counts in decoded save data v (a pointer) from
// writtenAt, the tick rate it was stamped with (0 or less: a file from before
// the stamp, written at config.DefaultTPS), to config.GetTargetTPS - the rate the
// game runs at and every save and chest file is stamped with - and returns
// that rate, the data's stamp from now on. Frame counts are each TickScaler
// and each signed integer (or map value) field tagged `ticks:"frames"`.
func RescaleSaved(v any, writtenAt int) int {
	live := config.GetTargetTPS()
	if from := normalTPS(writtenAt); from != live {
		w := ticksWalker{k: float64(live) / float64(from), holds: map[reflect.Type]bool{}}
		w.walk(reflect.ValueOf(v), false)
	}
	return live
}

// ScalesItself reports whether t's own ScaleTicks rescales a whole value of t
// (DualClock, items.RoundClock). A struct that embeds such a clock is not one:
// the clock scales as its field, and the struct's other fields are walked.
func ScalesItself(t reflect.Type) bool {
	if t.Kind() != reflect.Struct || !reflect.PointerTo(t).Implements(tickScalerType) {
		return false
	}
	for i := range t.NumField() {
		if f := t.Field(i); f.Anonymous && (f.Type.Implements(tickScalerType) || reflect.PointerTo(f.Type).Implements(tickScalerType)) {
			return false
		}
	}
	return true
}

type ticksWalker struct {
	k     float64
	holds map[reflect.Type]bool
}

func (w *ticksWalker) walk(v reflect.Value, frames bool) {
	if !frames && !w.holdsTicks(v.Type()) {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			w.walk(v.Elem(), frames)
		}
	case reflect.Struct:
		if !v.CanAddr() {
			return
		}
		if ScalesItself(v.Type()) {
			v.Addr().Interface().(TickScaler).ScaleTicks(w.k)
			return
		}
		t := v.Type()
		for i := range t.NumField() {
			if field := t.Field(i); field.IsExported() {
				w.walk(v.Field(i), field.Tag.Get("ticks") == "frames")
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			w.walk(v.Index(i), frames)
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(v.MapIndex(key))
			w.walk(elem, frames)
			v.SetMapIndex(key, elem)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if frames && v.CanSet() {
			v.SetInt(int64(mathutil.ScaleCount(int(v.Int()), w.k)))
		}
	}
}

// holdsTicks reports whether a value of type t can contain a frame count: a
// TickScaler or a tagged field somewhere below it. The walk skips every
// subtree that cannot, so a map of plain numbers is never rewritten.
func (w *ticksWalker) holdsTicks(t reflect.Type) bool {
	h, ok := w.holds[t]
	if !ok {
		h = reachesTicks(t, map[reflect.Type]bool{})
		w.holds[t] = h
	}
	return h
}

func reachesTicks(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return reachesTicks(t.Elem(), seen)
	case reflect.Struct:
		if reflect.PointerTo(t).Implements(tickScalerType) {
			return true
		}
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && (f.Tag.Get("ticks") == "frames" || reachesTicks(f.Type, seen)) {
				return true
			}
		}
	}
	return false
}
