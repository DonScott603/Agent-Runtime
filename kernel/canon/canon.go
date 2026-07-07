// Package canon implements canonical serialization per RFC-0001 §5 (D2):
// JSON, sorted keys, no insignificant whitespace, minimal escaping,
// UTF-8, strings NFC-normalized before serialization, integers only —
// floats are a hard error, not a conversion. The normative rules and
// golden cases live in docs/vectors/canon.json (_rules key).
//
// SECURITY-CRITICAL (CLAUDE.md): every hash ever computed depends on
// these bytes. Keep this file small enough to hand-review in full.
//
// Interpretations where D2 defers to JCS (RFC 8785), pending vectors:
//   - Object keys sort by UTF-16 code units (RFC 8785 §3.2.3), which
//     matches the roadmap E5 cross-check against an independent JCS
//     implementation. Byte order and UTF-16 order agree on ASCII.
//   - Minimal escaping per RFC 8785 §3.2.2.2: \" \\ \b \f \n \r \t,
//     other control chars as \u00xx lowercase hex, everything else
//     literal UTF-8.
//
// Deliberate deviations from JCS, mandated by D2:
//   - Numbers are integers only (int64/uint64 range); any float-typed
//     Go value or float-form token (., e, E) is ErrFloat.
//   - Strings (values and keys) NFC-normalize before serialization.
//
// Deterministic-by-construction rules:
//   - Invalid UTF-8 is an error, never replaced or passed through.
//   - Two keys that collide after NFC normalization are an error.
//   - Custom json.Marshaler implementations are refused (a marshaler
//     is uncontrolled nondeterminism); json.RawMessage is re-parsed
//     and re-canonicalized instead (idempotent on canonical input).
package canon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrFloat        = errors.New("canon: float in serialized value (RFC-0001 D2: integers only)")
	ErrInvalidUTF8  = errors.New("canon: string is not valid UTF-8")
	ErrDuplicateKey = errors.New("canon: duplicate object key after NFC normalization")
	ErrUnsupported  = errors.New("canon: unsupported type in serialized value")
)

var (
	rawMessageType = reflect.TypeOf(json.RawMessage(nil))
	jsonNumberType = reflect.TypeOf(json.Number(""))
	marshalerType  = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

// Canonical serializes v to its unique canonical byte sequence.
func Canonical(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := encode(&b, reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func encode(b *bytes.Buffer, v reflect.Value) error {
	// Unwrap interfaces and pointers; nil at any layer is null.
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}

	switch v.Type() {
	case rawMessageType:
		return encodeRaw(b, v.Bytes())
	case jsonNumberType:
		return encodeNumber(b, v.String())
	}
	if v.Type().Implements(marshalerType) || reflect.PointerTo(v.Type()).Implements(marshalerType) {
		return fmt.Errorf("%w: %s implements json.Marshaler (custom marshalers are nondeterminism)", ErrUnsupported, v.Type())
	}

	switch v.Kind() {
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		return ErrFloat
	case reflect.String:
		return encodeString(b, v.String())
	case reflect.Slice, reflect.Array:
		return encodeSequence(b, v)
	case reflect.Map:
		return encodeMap(b, v)
	case reflect.Struct:
		return encodeStruct(b, v)
	default:
		return fmt.Errorf("%w: kind %s", ErrUnsupported, v.Kind())
	}
	return nil
}

// encodeRaw re-parses embedded canonical bytes and re-canonicalizes
// them: identity on already-canonical input, deterministic otherwise.
func encodeRaw(b *bytes.Buffer, raw []byte) error {
	if raw == nil {
		b.WriteString("null")
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("canon: invalid json.RawMessage: %w", err)
	}
	if dec.More() {
		return errors.New("canon: trailing data in json.RawMessage")
	}
	return encode(b, reflect.ValueOf(v))
}

// encodeNumber accepts integer tokens in int64/uint64 range and
// re-formats them minimally. Float-form tokens are the hard error D2
// demands; integer tokens beyond uint64 are unsupported.
func encodeNumber(b *bytes.Buffer, s string) error {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		b.WriteString(strconv.FormatInt(i, 10))
		return nil
	}
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		b.WriteString(strconv.FormatUint(u, 10))
		return nil
	}
	if strings.ContainsAny(s, ".eE") {
		return fmt.Errorf("%w: number token %q", ErrFloat, s)
	}
	return fmt.Errorf("%w: number token %q outside int64/uint64 range", ErrUnsupported, s)
}

// encodeString NFC-normalizes then writes with minimal escaping
// (RFC 8785 §3.2.2.2). Escape decisions are all ASCII-range, so byte
// iteration is safe: multi-byte UTF-8 units are ≥ 0x80 and pass through.
func encodeString(b *bytes.Buffer, s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%w: %q", ErrInvalidUTF8, s)
	}
	s = norm.NFC.String(s)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			fmt.Fprintf(b, `\u%04x`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return nil
}

func encodeSequence(b *bytes.Buffer, v reflect.Value) error {
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
		return fmt.Errorf("%w: raw byte slice %s (binary belongs in blobs, RFC-0001 §2)", ErrUnsupported, v.Type())
	}
	if v.Kind() == reflect.Slice && v.IsNil() {
		b.WriteString("null") // mirror encoding/json: nil slice is null, empty slice is []
		return nil
	}
	b.WriteByte('[')
	for i := 0; i < v.Len(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := encode(b, v.Index(i)); err != nil {
			return err
		}
	}
	b.WriteByte(']')
	return nil
}

// member is one object key/value pair awaiting sorted emission.
type member struct {
	key string // NFC-normalized
	val reflect.Value
}

func emitMembers(b *bytes.Buffer, members []member) error {
	sort.Slice(members, func(i, j int) bool { return lessUTF16(members[i].key, members[j].key) })
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			if members[i-1].key == m.key {
				return fmt.Errorf("%w: %q", ErrDuplicateKey, m.key)
			}
			b.WriteByte(',')
		}
		if err := encodeString(b, m.key); err != nil {
			return err
		}
		b.WriteByte(':')
		if err := encode(b, m.val); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func encodeMap(b *bytes.Buffer, v reflect.Value) error {
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("%w: map key type %s (object keys are strings)", ErrUnsupported, v.Type().Key())
	}
	if v.IsNil() {
		b.WriteString("null") // mirror encoding/json: nil map is null, empty map is {}
		return nil
	}
	members := make([]member, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key().String()
		if !utf8.ValidString(k) {
			return fmt.Errorf("%w: object key %q", ErrInvalidUTF8, k)
		}
		members = append(members, member{key: norm.NFC.String(k), val: iter.Value()})
	}
	return emitMembers(b, members)
}

// encodeStruct serializes exported fields under their json tag names,
// sorted like any other object keys. Supported tag options: omitempty
// (encoding/json semantics). Anything fancier is refused — the frozen
// shapes in kernel/types.go need nothing more.
func encodeStruct(b *bytes.Buffer, v reflect.Value) error {
	t := v.Type()
	members := make([]member, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			return fmt.Errorf("%w: embedded field %s.%s", ErrUnsupported, t, f.Name)
		}
		if k := f.Type.Kind(); k == reflect.Float32 || k == reflect.Float64 {
			return fmt.Errorf("%w (field %s.%s)", ErrFloat, t, f.Name)
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" && opts == "" {
			continue
		}
		if opts != "" && opts != "omitempty" {
			return fmt.Errorf("%w: json tag option %q on %s.%s", ErrUnsupported, opts, t, f.Name)
		}
		fv := v.Field(i)
		if opts == "omitempty" && isEmpty(fv) {
			continue
		}
		if name == "" {
			name = f.Name
		}
		members = append(members, member{key: norm.NFC.String(name), val: fv})
	}
	return emitMembers(b, members)
}

// isEmpty mirrors encoding/json's omitempty emptiness.
func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// lessUTF16 orders keys by UTF-16 code units (RFC 8785 §3.2.3). This
// differs from byte order only outside ASCII; see package comment.
func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}
