package httpapi

import (
	"encoding/json"
	"maps"

	"github.com/aws/aws-lambda-go/events"
)

type Response struct {
	Status int
	Body   string
}

func JSON(status int, v any) Response {
	b, err := json.Marshal(v)
	if err != nil {
		return Error(500, "falha ao serializar resposta")
	}
	return Response{Status: status, Body: string(b)}
}

func Error(status int, msg string) Response {
	b, _ := json.Marshal(map[string]string{"erro": msg})
	return Response{Status: status, Body: string(b)}
}

var corsHeaders = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "Content-Type,Authorization",
	"Access-Control-Allow-Methods": "GET,POST,PUT,DELETE,OPTIONS",
}

var jsonHeaders = func() map[string]string {
	h := maps.Clone(corsHeaders)
	h["Content-Type"] = "application/json"
	h["Cache-Control"] = "no-store"
	return h
}()

func ToLambda(r Response) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: r.Status, Headers: jsonHeaders, Body: r.Body}
}
