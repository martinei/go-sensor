package instana

import (
	"net/http"
	"strconv"
	"strings"

	ot "github.com/opentracing/opentracing-go"
)

type textMapPropagator struct {
	tracer *tracerS
}

// Instana header constants
const (
	// FieldT Trace ID header
	FieldT = "x-instana-t"
	// FieldS Span ID header
	FieldS = "x-instana-s"
	// FieldL Level header
	FieldL = "x-instana-l"
	// FieldB OT Baggage header
	FieldB = "x-instana-b-"
)

func (r *textMapPropagator) inject(spanContext ot.SpanContext, opaqueCarrier interface{}) error {
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return ot.ErrInvalidSpanContext
	}

	roCarrier, ok := opaqueCarrier.(ot.TextMapReader)
	if !ok {
		return ot.ErrInvalidCarrier
	}

	// Handle pre-existing case-sensitive keys
	exstfieldT := FieldT
	exstfieldS := FieldS
	exstfieldL := FieldL
	exstfieldB := FieldB

	roCarrier.ForeachKey(func(k, v string) error {
		switch strings.ToLower(k) {
		case FieldT:
			exstfieldT = k
		case FieldS:
			exstfieldS = k
		case FieldL:
			exstfieldL = k
		default:
			if strings.HasPrefix(strings.ToLower(k), FieldB) {
				exstfieldB = string([]rune(k)[:len(FieldB)])
			}
		}
		return nil
	})

	carrier, ok := opaqueCarrier.(ot.TextMapWriter)
	if !ok {
		return ot.ErrInvalidCarrier
	}

	if c, ok := opaqueCarrier.(ot.HTTPHeadersCarrier); ok {
		// Even though the godoc claims that the key passed to (*http.Header).Set()
		// is case-insensitive, it actually normalizes it using textproto.CanonicalMIMEHeaderKey()
		// before populating the value. As a result headers with non-canonical will not be
		// overwritted with a new value. This is only the case if header names were set while
		// initializing the http.Header instance, i.e.
		//     h := http.Headers{"X-InStAnA-T": {"abc123"}}
		// and does not apply to a common case when requests are being created using http.NewRequest()
		// or http.ReadRequest() that call (*http.Header).Set() to set header values.
		y := http.Header(c)
		delete(y, exstfieldT)
		delete(y, exstfieldS)
		delete(y, exstfieldL)

		for key := range y {
			if strings.HasPrefix(strings.ToLower(key), FieldB) {
				delete(y, key)
			}
		}
	}

	carrier.Set(exstfieldT, FormatID(sc.TraceID))
	carrier.Set(exstfieldS, FormatID(sc.SpanID))
	carrier.Set(exstfieldL, strconv.Itoa(1))

	for k, v := range sc.Baggage {
		carrier.Set(exstfieldB+k, v)
	}
	return nil
}

func (r *textMapPropagator) extract(opaqueCarrier interface{}) (ot.SpanContext, error) {
	carrier, ok := opaqueCarrier.(ot.TextMapReader)
	if !ok {
		return nil, ot.ErrInvalidCarrier
	}

	spanContext := SpanContext{
		Baggage: make(map[string]string),
	}

	var fieldCount int
	err := carrier.ForeachKey(func(k, v string) error {
		switch strings.ToLower(k) {
		case FieldT:
			fieldCount++

			traceID, err := ParseID(v)
			if err != nil {
				return ot.ErrSpanContextCorrupted
			}

			spanContext.TraceID = traceID
		case FieldS:
			fieldCount++

			spanID, err := ParseID(v)
			if err != nil {
				return ot.ErrSpanContextCorrupted
			}

			spanContext.SpanID = spanID
		default:
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, FieldB) {
				spanContext.Baggage[strings.TrimPrefix(lk, FieldB)] = v
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if fieldCount == 0 {
		return nil, ot.ErrSpanContextNotFound
	} else if fieldCount < 2 {
		return nil, ot.ErrSpanContextCorrupted
	}

	return spanContext, nil
}
