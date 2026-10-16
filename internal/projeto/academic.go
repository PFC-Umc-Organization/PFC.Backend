package projeto

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// UseAcademic wires the academic module in; the groups module replaces this
// package in phase 3.
func UseAcademic(a *academic.Service) {
	turmaDoRGM = a.CohortOf
	turmasDosRGMs = a.CohortsOf
	programaDaTurma = a.ProgramOfCohort
	programaExiste = a.ProgramExists
	matriculado = a.IsEnrolled
}

type Projects struct{}

func (Projects) ProgramHasProjects(ctx context.Context, programID string) (bool, error) {
	out, err := common.DynamoDB().Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(common.Tabela),
		IndexName:                 aws.String(common.Indice),
		KeyConditionExpression:    aws.String("GSI1PK = :programa"),
		ExpressionAttributeValues: common.Item{":programa": common.S("PROGRAM#" + programID)},
		ProjectionExpression:      aws.String("PK"),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}
