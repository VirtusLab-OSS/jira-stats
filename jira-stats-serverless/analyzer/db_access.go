package analyzer

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"github.com/ztrue/tracerr"
	"jira-stats/jira-stats-serverless/analyzer/domain"
	"log"
	"time"
)

const CONFIG_TABLE = "Config"
const TICKET_TABLE = "Ticket"

// Fetch all tickets that had dev start time before given date

func fetchTicketsWithDevStartTimeBefore(devStartDate time.Time, devEndDate time.Time) ([]domain.Ticket, error) {
	defer timeTrack(time.Now(), fmt.Sprintf("DB scan for (%s, %s)", devStartDate.Format(time.RFC3339), devEndDate.Format(time.RFC3339)))

	sess := session.Must(session.NewSession())
	svc := dynamodb.New(sess)

	filter :=
		expression.Or(
			expression.Or( // tickets that had dev time contained or overlapping with searched interval
				expression.Between(expression.Name("DevStartDate"), expression.Value(devStartDate.Unix()), expression.Value(devEndDate.Unix())),
				expression.Between(expression.Name("DevEndDate"), expression.Value(devStartDate.Unix()), expression.Value(devEndDate.Unix())),
			),
			expression.And( // tickets that had dev time containing searched interval
				expression.LessThanEqual(expression.Name("DevStartDate"), expression.Value(devStartDate.Unix())),
				expression.GreaterThanEqual(expression.Name("DevEndDate"), expression.Value(devEndDate.Unix())),
			),
		)

	expr, err := expression.NewBuilder().
		//WithKeyCondition(keyCondition).
		WithFilter(filter).
		Build()

	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	queryInput := dynamodb.ScanInput{
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		TableName:                 aws.String(TICKET_TABLE),
	}

	queryResults, err := svc.Scan(&queryInput)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	tickets := make([]domain.Ticket, 0)
	for _, result := range queryResults.Items {
		var ticket domain.Ticket
		err := dynamodbattribute.UnmarshalMap(result, &ticket)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}

		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// Adds new ticket representation to db, overwrites previously existing one
func store(ticket domain.Ticket) error {
	sess := session.Must(session.NewSession())

	err := delete(sess, ticket.Id)
	if err != nil {
		return tracerr.Wrap(err)
	}

	err = insert(sess, ticket)
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

func delete(sess *session.Session, ticketId string) error {
	svc := dynamodb.New(sess)

	input := dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"Id": {
				S: aws.String(ticketId),
			},
		},
		TableName: aws.String(TICKET_TABLE),
	}

	_, err := svc.DeleteItem(&input)
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

func insert(sess *session.Session, ticket domain.Ticket) error {
	svc := dynamodb.New(sess)

	item, err := dynamodbattribute.MarshalMap(ticket)
	if err != nil {
		return tracerr.Wrap(err)
	}

	input := dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String(TICKET_TABLE),
	}
	_, err = svc.PutItem(&input)
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

func storeLastUpdate(updateTime time.Time) error {
	sess := session.Must(session.NewSession())
	svc := dynamodb.New(sess)

	prevUpdate, err := getLastUpdate()
	if err != nil {
		return tracerr.Wrap(err)
	}

	if prevUpdate != domain.BEGINING_OF_TIME {
		input := &dynamodb.UpdateItemInput{
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":w": {
					S: aws.String(updateTime.Format(time.RFC3339)),
				},
			},
			Key: map[string]*dynamodb.AttributeValue{
				"ConfigName": {
					S: aws.String("LastUpdate"),
				},
			},
			ReturnValues:     aws.String("UPDATED_NEW"),
			TableName:        aws.String(CONFIG_TABLE),
			UpdateExpression: aws.String("set ConfigValue = :w"),
		}
		output, err := svc.UpdateItem(input)
		if err != nil {
			return tracerr.Wrap(err)
		}
		log.Printf("Output is: %s", output)
	} else {
		configItem := domain.ConfigItem{
			ConfigName:  "LastUpdate",
			ConfigValue: updateTime.Format(time.RFC3339),
		}

		item, err := dynamodbattribute.MarshalMap(configItem)
		if err != nil {
			return tracerr.Wrap(err)
		}

		input := dynamodb.PutItemInput{
			Item:      item,
			TableName: aws.String(CONFIG_TABLE),
		}

		_, err = svc.PutItem(&input)
		if err != nil {
			return tracerr.Wrap(err)
		}
	}

	return err
}

func getLastUpdate() (time.Time, error) {
	sess := session.Must(session.NewSession())
	svc := dynamodb.New(sess)

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"ConfigName": {
				S: aws.String("LastUpdate"),
			},
		},

		TableName: aws.String(CONFIG_TABLE),
	})
	if err != nil {
		return time.Now(), tracerr.Wrap(err)
	}

	configResult := domain.ConfigItem{}
	err = dynamodbattribute.UnmarshalMap(result.Item, &configResult)
	if err != nil {
		return time.Now(), tracerr.Wrap(err)
	}

	lastUpdate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if configResult.ConfigValue != "" {
		lastUpdate, err = time.Parse(time.RFC3339, configResult.ConfigValue)
		if err != nil {
			return time.Now(), tracerr.Wrap(err)
		}
	}

	return lastUpdate, nil
}
