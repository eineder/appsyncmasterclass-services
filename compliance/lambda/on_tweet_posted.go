package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	ls "github.com/aws/aws-sdk-go/service/lambda"
)

type OnTweetPostedEvent struct {
	Tweet           Tweet  `json:"detail"`
	TweetsTableName string `json:"tweetsTableName"`
}

type Tweet struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

type OnTweetPostedResponse struct {
	Tweet               Tweet    `json:"tweet"`
	ContainedSwearwords []string `json:"containedSwearwords"`
}

type ContainsSwearwordsResponse struct {
	Swearwords []string `json:"swearwords"`
}

func HandleRequest(ctx context.Context, event *OnTweetPostedEvent) (*OnTweetPostedResponse, error) {
	if event == nil {
		fmt.Println("Received nil event")
		return nil, fmt.Errorf("received nil event")
	}

	if event.TweetsTableName == "" {
		fmt.Println("Received empty tweetsTableName")
		return nil, fmt.Errorf("received empty tweetsTableName")
	}

	lambdaName := aws.String(os.Getenv("SWEARWORDS_LAMBDA_NAME"))
	cfg := aws.NewConfig()
	sess, sessErr := session.NewSession(cfg)
	if sessErr != nil {
		fmt.Println("Error creating session ", sessErr)
		return nil, sessErr
	}

	client := ls.New(sess)
	payloadStruct := struct {
		Text string `json:"text"`
	}{
		Text: event.Tweet.Text,
	}
	payload, marshalErr := json.Marshal(payloadStruct)
	if marshalErr != nil {
		fmt.Println("Error marshalling payload ", marshalErr)
		return nil, marshalErr
	}

	response, lambdaErr := client.Invoke(&ls.InvokeInput{
		FunctionName: lambdaName,
		Payload:      payload,
	})
	if lambdaErr != nil {
		fmt.Println("Error calling lambda ", lambdaErr)
		return nil, lambdaErr
	}

	fmt.Printf("%+v\n", response)
	var containsSwearwordsResponse ContainsSwearwordsResponse
	unmarErr := json.Unmarshal(response.Payload, &containsSwearwordsResponse)
	if unmarErr != nil {
		fmt.Println("Error unmarshalling response ", unmarErr)
		return nil, unmarErr
	}

	if containsSwearwordsResponse.Swearwords != nil && len(containsSwearwordsResponse.Swearwords) > 0 {
		fmt.Println("Tweet contains swearwords", event.Tweet)
		markErr := markTweetAsNonCompliant(event.Tweet.Id, event.TweetsTableName, sess)
		if markErr != nil {
			fmt.Println("Error marking tweet as non-compliant ", markErr)
			return nil, markErr
		}
		fmt.Printf("Marked tweet as non-compliant. Text: '%s', Swearwords: '%+v'", event.Tweet, containsSwearwordsResponse.Swearwords)
	}

	return &OnTweetPostedResponse{
		Tweet:               event.Tweet,
		ContainedSwearwords: containsSwearwordsResponse.Swearwords,
	}, nil
}

func markTweetAsNonCompliant(tweetId string, tableName string, sess *session.Session) error {
	dynamoClient := dynamodb.New(sess)
	output, err := dynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(tweetId),
			},
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":c": {
				BOOL: aws.Bool(false),
			},
		},
		UpdateExpression:    aws.String("SET nonCompliant = :c"),
		ConditionExpression: aws.String("attribute_exists(id)"),
	})

	if err != nil {
		fmt.Println("Error updating item ", err)
		return err
	}

	fmt.Println("Updated item ", output)

	return nil
}

func main() {
	lambda.Start(HandleRequest)
}
