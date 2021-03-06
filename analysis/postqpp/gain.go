package postqpp

import (
	"github.com/hscells/groove/analysis"
	"github.com/hscells/groove/pipeline"
	"github.com/hscells/groove/stats"
	"github.com/hscells/trecresults"
	"math"
)

type weightedInformationGain struct{}
type weightedExpansionGain struct{}

var (
	// WeightedInformationGain aims to measure the weighted entropy of the top k ranked documents.
	WeightedInformationGain = weightedInformationGain{}
	// WeightedExpansionGain aims to analyse the quality of retrieved pseudo relevant documents by measuring the
	// likelihood that they will have topic drift.
	WeightedExpansionGain = weightedExpansionGain{}
)

func (wig weightedInformationGain) Name() string {
	return "WIG"
}

func (wig weightedInformationGain) Execute(q pipeline.Query, s stats.StatisticsSource) (float64, error) {
	queryLength := float64(len(analysis.QueryTerms(q.Query)))
	results, err := s.Execute(q, s.SearchOptions())
	if err != nil {
		return 0.0, err
	}
	if len(results) == 0 {
		return 0.0, nil
	}
	D := results[len(results)-1].Score
	totalScore := 0.0

	k := s.Parameters()["k"]
	if float64(len(results)) < k {
		k = float64(len(results))
	}
	if k < 1 {
		k = 1
	}

	for _, result := range results {
		d := result.Score
		totalScore += (1.0 / math.Sqrt(queryLength)) * (d - D)
	}

	return (1.0 / k) * totalScore, nil
}

func (weg weightedExpansionGain) Name() string {
	return "WEG"
}

func (weg weightedExpansionGain) cnprf(k float64, results trecresults.ResultList) float64 {
	n := len(results) - int(k)
	nprf := 0.0
	for _, result := range results[n:] {
		nprf += result.Score
	}
	return nprf / float64(len(results[n:]))
}

func (weg weightedExpansionGain) Execute(q pipeline.Query, s stats.StatisticsSource) (float64, error) {
	queryLength := float64(len(analysis.QueryTerms(q.Query)))
	results, err := s.Execute(q, s.SearchOptions())
	if err != nil {
		return 0.0, err
	}
	if len(results) == 0 {
		return 0.0, nil
	}

	k := s.Parameters()["k"]
	if float64(len(results)) < k {
		k = float64(len(results))
	}
	if k < 1 {
		k = 1
	}

	D := weg.cnprf(k, results)
	totalScore := 0.0

	for _, result := range results {
		d := result.Score
		totalScore += (1.0 / math.Sqrt(queryLength)) * (d - D)
	}

	return (1.0 / k) * totalScore, nil
}
