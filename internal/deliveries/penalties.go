package deliveries

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxPenalties active penalties fail the group: it can no longer submit
// until the academic team waives one.
const MaxPenalties = 3

type PenaltySource string

const (
	PenaltyDeadline PenaltySource = "PRAZO"
	PenaltyManual   PenaltySource = "MANUAL"
)

const (
	deadlinePenaltyPrefix = "prazo-"
	manualPenaltyPrefix   = "manual-"
	maxReasonLen          = 500
)

type Waiver struct {
	Reason string `json:"motivo"`
	By     string `json:"por"`
	At     string `json:"em"`
}

// Penalty is active while Waiver is nil. A deadline penalty is not stored:
// it exists while the activity is closed without the group's submission.
type Penalty struct {
	ID            string        `json:"id"`
	Source        PenaltySource `json:"origem"`
	ActivityID    string        `json:"atividadeId,omitempty"`
	ActivityTitle string        `json:"atividadeTitulo,omitempty"`
	Reason        string        `json:"motivo,omitempty"`
	AppliedAt     string        `json:"aplicadaEm"`
	AppliedBy     string        `json:"aplicadaPor,omitempty"`
	Waiver        *Waiver       `json:"perdao"`
}

type Penalties struct {
	ProjectID string    `json:"projetoId"`
	Active    int       `json:"ativas"`
	Limit     int       `json:"limite"`
	Failed    bool      `json:"reprovado"`
	Items     []Penalty `json:"penalidades"`
}

type PenaltyInput struct {
	Reason     string `json:"motivo"`
	ActivityID string `json:"atividadeId"`
}

type WaiverInput struct {
	Reason string `json:"motivo"`
}

// penaltyRecord is what the store keeps: a manual penalty, or the waiver of
// a deadline penalty (then only ID and Waiver are set).
type penaltyRecord struct {
	ID         string
	ActivityID string
	Reason     string
	AppliedAt  string
	AppliedBy  string
	Waiver     *Waiver
}

func validReason(reason string) (string, string) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", msgReasonRequired
	case utf8.RuneCountInString(reason) > maxReasonLen:
		return "", msgReasonTooLong
	}
	return reason, ""
}

func penaltiesOf(projectID string, activities []Activity, submissions []Submission, records []penaltyRecord, now time.Time) Penalties {
	submitted := map[string]bool{}
	for _, s := range submissions {
		submitted[s.ActivityID] = true
	}
	waivers := map[string]*Waiver{}
	titles := map[string]string{}
	for _, a := range activities {
		titles[a.ID] = a.Title
	}
	out := Penalties{ProjectID: projectID, Limit: MaxPenalties, Items: []Penalty{}}
	for _, r := range records {
		if !strings.HasPrefix(r.ID, manualPenaltyPrefix) {
			waivers[r.ID] = r.Waiver
			continue
		}
		out.Items = append(out.Items, Penalty{
			ID: r.ID, Source: PenaltyManual, ActivityID: r.ActivityID, ActivityTitle: titles[r.ActivityID],
			Reason: r.Reason, AppliedAt: r.AppliedAt, AppliedBy: r.AppliedBy, Waiver: r.Waiver,
		})
	}
	for _, a := range activities {
		deadline, ok := deadlineOf(a.Deadline)
		if !ok || !now.After(deadline) || submitted[a.ID] {
			continue
		}
		id := deadlinePenaltyPrefix + a.ID
		out.Items = append(out.Items, Penalty{
			ID: id, Source: PenaltyDeadline, ActivityID: a.ID, ActivityTitle: a.Title,
			AppliedAt: deadline.UTC().Format(time.RFC3339), Waiver: waivers[id],
		})
	}
	slices.SortStableFunc(out.Items, func(a, b Penalty) int { return strings.Compare(a.AppliedAt, b.AppliedAt) })
	for _, p := range out.Items {
		if p.Waiver == nil {
			out.Active++
		}
	}
	out.Failed = out.Active >= MaxPenalties
	return out
}
