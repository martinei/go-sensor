// (c) Copyright IBM Corp. 2021
// (c) Copyright Instana Inc. 2021

// Package instaawssdk instruments github.com/aws/aws-sdk-go

package instaawssdk

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sqs"
	instana "github.com/instana/go-sensor"
)

var errMethodNotInstrumented = errors.New("method not instrumented")

// InstrumentSession instruments github.com/aws/aws-sdk-go/aws/session.Session by
// injecting handlers to create and finalize Instana spans
func InstrumentSession(sess *session.Session, sensor *instana.Sensor) {
	sess.Handlers.Validate.PushBack(func(req *request.Request) {
		switch req.ClientInfo.ServiceName {
		case s3.ServiceName:
			StartS3Span(req, sensor)
		case sqs.ServiceName:
			StartSQSSpan(req, sensor)
		case dynamodb.ServiceName:
			StartDynamoDBSpan(req, sensor)
		}
	})

	sess.Handlers.Complete.PushBack(func(req *request.Request) {
		switch req.ClientInfo.ServiceName {
		case s3.ServiceName:
			FinalizeS3Span(req)
		case sqs.ServiceName:
			FinalizeSQSSpan(req)

			if data, ok := req.Data.(*sqs.ReceiveMessageOutput); ok {
				params, ok := req.Params.(*sqs.ReceiveMessageInput)
				if !ok {
					sensor.Logger().Error(fmt.Sprintf("unexpected SQS ReceiveMessage parameters type: %T", req.Params))
					break
				}

				for i := range data.Messages {
					sp := TraceSQSMessage(data.Messages[i], sensor)
					sp.SetTag("sqs.queue", aws.StringValue(params.QueueUrl))
					sp.Finish()
				}
			}
		case dynamodb.ServiceName:
			FinalizeDynamoDBSpan(req)
		}
	})
}
