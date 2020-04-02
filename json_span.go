package instana

// Span represents the OpenTracing span document to be sent to the agent
type Span struct {
	TraceID   int64       `json:"t"`
	ParentID  int64      `json:"p,omitempty"`
	SpanID    int64       `json:"s"`
	Timestamp uint64      `json:"ts"`
	Duration  uint64      `json:"d"`
	Name      string      `json:"n"`
	From      *fromS      `json:"f"`
	Kind      int         `json:"k"`
	Error     bool        `json:"error"`
	Ec        int         `json:"ec,omitempty"`
	Lang      string      `json:"ta,omitempty"`
	Data      SDKSpanData `json:"data"`
}

// SpanData contains fields to be sent in the `data` section of an OT span document. These fields are
// common for all span types.
type SpanData struct {
	Service string `json:"service,omitempty"`
}

// NewSpanData initializes a new span data from tracer span
func NewSpanData(span *spanS) SpanData {
	return SpanData{Service: span.Service}
}

// SDKSpanData represents the `data` section of an SDK span sent within an OT span document
type SDKSpanData struct {
	SpanData
	Tags SDKSpanTags `json:"sdk"`
}

// NewSDKSpanData initializes a new SDK span data from tracer span
func NewSDKSpanData(span *spanS) SDKSpanData {
	return SDKSpanData{
		SpanData: NewSpanData(span, SDKSpanType),
		Tags:     NewSDKSpanTags(span),
	}
}

// SDKSpanTags contains fields within the `data.sdk` section of an OT span document
type SDKSpanTags struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Return    string                 `json:"return,omitempty"`
	Custom    map[string]interface{} `json:"custom,omitempty"`
}

// NewSDKSpanTags extracts SDK span tags from a tracer span
func NewSDKSpanTags(span *spanS) SDKSpanTags {
	tags := SDKSpanTags{
		Name:   span.Operation,
		Type:   span.Kind().String(),
		Custom: map[string]interface{}{},
	}

	if len(span.Tags) != 0 {
		tags.Custom["tags"] = span.Tags
	}

	if logs := span.collectLogs(); len(logs) > 0 {
		tags.Custom["logs"] = logs
	}

	if len(span.context.Baggage) != 0 {
		tags.Custom["baggage"] = span.context.Baggage
	}

	return tags
}
