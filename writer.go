package main

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

type Writer struct {
	conn              *cloudwatchlogs.CloudWatchLogs
	logGroupName      string
	logStreamName     string
	nextSequenceToken string
}

func NewWriter(sess *awsSession.Session, logGroupName, logStreamName, firstSeqToken string) (*Writer, error) {
	conn := cloudwatchlogs.New(sess)

	return &Writer{
		conn:              conn,
		logGroupName:      logGroupName,
		logStreamName:     logStreamName,
		nextSequenceToken: firstSeqToken,
	}, nil
}

func (w *Writer) WriteBatch(records []Record) (string, error) {

	events := make([]*cloudwatchlogs.InputLogEvent, 0, len(records))
	for _, record := range records {
		jsonDataBytes, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return "", err
		}
		jsonData := string(jsonDataBytes)

		events = append(events, &cloudwatchlogs.InputLogEvent{
			Message:   aws.String(jsonData),
			Timestamp: aws.Int64(int64(record.TimeUsec)),
		})
	}

	putEvents := func() error {
		request := &cloudwatchlogs.PutLogEventsInput{
			LogEvents:     events,
			LogGroupName:  &w.logGroupName,
			LogStreamName: &w.logStreamName,
		}
		if w.nextSequenceToken != "" {
			request.SequenceToken = aws.String(w.nextSequenceToken)
		}
		result, err := w.conn.PutLogEvents(request)
		if err != nil {
			return err
		}
		w.nextSequenceToken = *result.NextSequenceToken
		return nil
	}

	createStream := func() error {
		request := &cloudwatchlogs.CreateLogStreamInput{
			LogGroupName:  &w.logGroupName,
			LogStreamName: &w.logStreamName,
		}
		_, err := w.conn.CreateLogStream(request)
		return err
	}

	err := putEvents()
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "ResourceNotFoundException" {
				// Maybe our log stream doesn't exist yet. We'll try
				// to create it and then, if we're successful, try
				// writing the events again.
				err := createStream()
				if err != nil {
					return "", fmt.Errorf("failed to create stream: %s", err)
				}

				err = putEvents()
				if err != nil {
					return "", err
				}
			}
		} else {
			return "", err
		}
	}

	return w.nextSequenceToken, nil
}
