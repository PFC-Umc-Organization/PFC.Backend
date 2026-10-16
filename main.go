package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/app"
)

func main() {
	deps, err := app.Load(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	lambda.Start(app.New(deps).Lambda)
}
