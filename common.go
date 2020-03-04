package stdchat

import (
	"net/url"
	"path"
	"strings"
	"time"
)

// IDInfo has information on an object's ID and name.
// Name and DisplayName are both optional, they fallback to Name and ID if empty.
type IDInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`     // use GetName
	DisplayName string `json:"dispName,omitempty"` // use GetDisplayName
}

// SetName sets the name and display name. Set the ID before calling this.
func (x *IDInfo) SetName(name, displayName string) {
	if x.ID != name {
		x.Name = name
	} else {
		x.Name = ""
	}
	if x.GetName() != displayName {
		x.DisplayName = displayName
	} else {
		x.DisplayName = ""
	}
}

// GetID gets the unique identifier.
func (x IDInfo) GetID() string {
	return x.ID
}

// GetName gets the name (or ID if no name)
func (x IDInfo) GetName() string {
	if x.Name != "" {
		return x.Name
	}
	return x.ID
}

// GetName gets the display name (or name if no display name)
func (x IDInfo) GetDisplayName() string {
	if x.DisplayName != "" {
		return x.DisplayName
	}
	return x.GetName()
}

// TypeInfo has information on the type of an object.
type TypeInfo struct {
	Type string `json:"type"`
}

// GetType gets the type.
func (x TypeInfo) GetType() string {
	return x.Type
}

// IsType returns true if this object is of type part.
func (x TypeInfo) IsType(part string) bool {
	return IsType(x.Type, part)
}

// IsType returns true if msgType is of type part.
func IsType(msgType, part string) bool {
	return part == msgType ||
		len(part) < len(msgType) &&
			msgType[len(part)] == '/' && part == msgType[:len(part)]
}

// EntityInfo represents an entity with an ID, optional name and a type.
type EntityInfo struct {
	IDInfo
	TypeInfo // the type of entity, like: net, user, server, ...
}

func (x *EntityInfo) Init(id, typ string) {
	x.ID = id
	x.Type = typ
}

// Message is a message in a specific MIME type.
// Text types are assumed to be in UTF-8 unless otherwise specified.
type Message struct {
	TypeInfo        // MIME type
	Content  string `json:"content"`
}

func (msg *Message) SetText(content string) {
	msg.Type = "text/plain"
	msg.Content = content
}

func (msg Message) Valid() bool {
	return msg.Type != ""
}

// MessageInfo represents a message in various types/formats.
type MessageInfo []Message

func (msg *MessageInfo) Set(msgType, content string) {
	for i, x := range *msg {
		if x.Type == msgType {
			x.Content = content
			(*msg)[i] = x
			return
		}
	}
	x := Message{}
	x.Type = msgType
	x.Content = content
	*msg = append(*msg, x)
}

// SetText in text/plain type.
func (msg *MessageInfo) SetText(content string) {
	msg.Set("text/plain", content)
}

// Get message in the specified type, or an empty message.
func (msg MessageInfo) Get(msgType string) Message {
	for _, x := range msg {
		if x.Type == msgType {
			return x
		}
	}
	return Message{}
}

// String returns the content for text/plain, or empty string.
func (msg MessageInfo) String() string {
	return msg.Get("text/plain").Content
}

// KeyValueInfo represents a key-value pair.
// [0] is the key name, [1] is the value.
type KeyValueInfo [2]string

// Key
func (kvi KeyValueInfo) Key() string {
	return kvi[0]
}

// Value
func (kvi KeyValueInfo) Value() string {
	return kvi[1]
}

// ValuesInfo represents zero or more auxiliary key-value pairs.
// Protocol-specific keys are PROTO.* and adhere to that protocol.
// Implementation-specific keys are x-NAME.* or PROTO.x-NAME.* (depending on scope)
// Duplicate keys are valid.
type ValuesInfo []KeyValueInfo

// Lookup the value of the specified key, returns true if found.
func (vi ValuesInfo) Lookup(key string) (string, bool) {
	for _, x := range vi {
		if x.Key() == key {
			return x.Value(), true
		}
	}
	return "", false
}

// Get the value of the specified key, or empty string.
func (vi ValuesInfo) Get(key string) string {
	s, _ := vi.Lookup(key)
	return s
}

// AppendAll appends all the values for the specified key and returns the updates.
func (vi ValuesInfo) AppendAll(result []string, key string) []string {
	for _, x := range vi {
		if x.Key() == key {
			result = append(result, x.Value())
		}
	}
	return result
}

// GetAll returns all the values for the specified key, or nil if none.
func (vi ValuesInfo) GetAll(key string) []string {
	return vi.AppendAll(nil, key)
}

// Set will update the value for an existing key, or add the new key-value pair.
// If more than one matching key exists, updates only the first and ignores the rest.
func (vi *ValuesInfo) Set(key, value string) *ValuesInfo {
	for i, x := range *vi {
		if x.Key() == key {
			(*vi)[i][1] = value
			return vi
		}
	}
	return vi.Add(key, value)
}

// Add the specified key and value, regardless if the key exists or not.
func (vi *ValuesInfo) Add(key, value string) *ValuesInfo {
	*vi = append(*vi, KeyValueInfo{key, value})
	return vi
}

// MediaInfo has info related to media, such as an image or video.
type MediaInfo struct {
	TypeInfo            // MIME type, or hint such as "image/*"
	Name     string     `json:"name,omitempty"` // Default is file name from URL.
	URL      string     `json:"url"`
	ThumbURL string     `json:"thumb,omitempty"`   // optional thumbnail.
	Expires  time.Time  `json:"expires,omitempty"` // When the URL expires.
	Values   ValuesInfo `json:"values,omitempty"`
}

func (x *MediaInfo) Init(typ, url string) {
	x.Type = typ
	x.URL = url
}

func (x MediaInfo) getURLName() string {
	q := x.URL
	iend := strings.IndexAny(q, "#?")
	if iend != -1 {
		q = q[:iend]
	}
	b := path.Base(q)
	if b != "." && b != "/" {
		bu, _ := url.QueryUnescape(b)
		if bu != "" {
			return bu
		}
		return b
	}
	return x.URL
}

func (x MediaInfo) GetID() string {
	return x.URL
}

func (x MediaInfo) GetName() string {
	if x.Name != "" {
		return x.Name
	}
	return x.getURLName()
}

func (x MediaInfo) GetDisplayName() string {
	return x.GetName()
}

func (x *MediaInfo) MarshalJSON() ([]byte, error) {
	if x.URL == "" {
		return []byte("null"), nil
	}
	type m *MediaInfo // Bypass MarshalJSON recursion.
	return json.Marshal(m(x))
}
