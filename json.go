package stdchat

import (
	"time"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.Config{
	AllowOmitEmptyStruct: true,
	SortMapKeys:          true,
	EscapeHTML:           true,
}.Froze()

type basicJSONer interface {
	Marshal(v interface{}) ([]byte, error)
	MarshalIndent(v interface{}, prefix, indent string) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
}

// JSON encoder/decoder. Marshal, MarshalIndent, Unmarshal.
var JSON basicJSONer = json

func init() {
	jsoniter.RegisterTypeEncoder("time.Time", &timeOmitZero{})
	jsoniter.RegisterTypeDecoder("time.Time", &timeOmitZero{})
}

type timeOmitZero struct {
}

func (codec *timeOmitZero) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	t, err := time.Parse(time.RFC3339, iter.ReadString())
	if err != nil {
		iter.ReportError("unmarshal", err.Error())
		return
	}
	*((*time.Time)(ptr)) = t
}

func (codec *timeOmitZero) IsEmpty(ptr unsafe.Pointer) bool {
	ts := *((*time.Time)(ptr))
	return ts.IsZero()
}

func (codec *timeOmitZero) Encode(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	ts := *((*time.Time)(ptr))
	b, err := ts.MarshalJSON()
	if err != nil {
		stream.Error = err
		return
	}
	stream.Write(b)
}
