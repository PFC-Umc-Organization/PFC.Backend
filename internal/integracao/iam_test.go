package integracao

import (
	"context"
	"fmt"
	"sort"
	"sync"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go/middleware"
)

// lambdaPolicy mirrors the IAM policy of the Lambda in PFC.Infra
// (modules/backend-lambda/data.tf). DynamoDB Local checks no permission, so
// an operation the policy lacks passes every test here and fails with 500 in
// AWS — which is how deleting a cohort broke after ConditionCheckItem and
// BatchGetItem came in.
var lambdaPolicy = map[string]bool{
	"dynamodb:GetItem": true, "dynamodb:PutItem": true, "dynamodb:UpdateItem": true,
	"dynamodb:DeleteItem": true, "dynamodb:Query": true,
	"dynamodb:BatchWriteItem": true, "dynamodb:BatchGetItem": true, "dynamodb:ConditionCheckItem": true,
	"cognito-idp:ListUsers": true, "cognito-idp:AdminCreateUser": true,
	"s3:PutObject": true, "s3:GetObject": true, "s3:DeleteObject": true, "s3:ListBucket": true,
}

// s3Actions maps operations to their IAM action. Presigning runs the
// operation's middleware too, so PutObject and GetObject are recorded when
// the URLs are signed; the browser's request uses the Lambda's permission.
var s3Actions = map[string]string{
	"HeadObject": "s3:GetObject", "ListObjectsV2": "s3:ListBucket", "DeleteObjects": "s3:DeleteObject",
}

// Cognito's public operations are authorized by the app client, not by IAM.
var cognitoPublic = map[string]bool{
	"InitiateAuth": true, "SignUp": true, "ConfirmSignUp": true, "ResendConfirmationCode": true,
	"ForgotPassword": true, "ConfirmForgotPassword": true, "RespondToAuthChallenge": true,
}

type actionRecorder struct {
	mu   sync.Mutex
	used map[string]bool
}

var recorder = &actionRecorder{used: map[string]bool{}}

func (r *actionRecorder) add(action string) {
	r.mu.Lock()
	r.used[action] = true
	r.mu.Unlock()
}

// middleware records the IAM action of every call. TransactWriteItems is not
// an action of its own: each item needs the action of its kind.
func (r *actionRecorder) middleware(prefix string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Initialize.Add(middleware.InitializeMiddlewareFunc("recordIAMAction",
			func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
				op := awsmiddleware.GetOperationName(ctx)
				switch t := in.Parameters.(type) {
				case *dynamodb.TransactWriteItemsInput:
					for _, it := range t.TransactItems {
						switch {
						case it.Put != nil:
							r.add("dynamodb:PutItem")
						case it.Update != nil:
							r.add("dynamodb:UpdateItem")
						case it.Delete != nil:
							r.add("dynamodb:DeleteItem")
						case it.ConditionCheck != nil:
							r.add("dynamodb:ConditionCheckItem")
						}
					}
				default:
					if a, ok := s3Actions[op]; ok && prefix == "s3" {
						r.add(a)
					} else if !(prefix == "cognito-idp" && cognitoPublic[op]) {
						r.add(prefix + ":" + op)
					}
				}
				return next.HandleInitialize(ctx, in)
			}), middleware.After)
	}
}

func (r *actionRecorder) missing() []string {
	var out []string
	for a := range r.used {
		if !lambdaPolicy[a] {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

func checkIAMActions() error {
	if m := recorder.missing(); len(m) > 0 {
		return fmt.Errorf("o backend usa ações que a política da Lambda não libera: %v — acrescente em PFC.Infra/modules/backend-lambda/data.tf e em lambdaPolicy (iam_test.go)", m)
	}
	return nil
}
