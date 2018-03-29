package rewrite

import (
	"fmt"
	"github.com/hscells/cqr"
	"github.com/hscells/groove"
	"io"
	"sort"
	"github.com/hscells/groove/stats"
	"github.com/hscells/groove/analysis/preqpp"
	"github.com/hscells/groove/analysis"
)

// Feature is some value that is applicable to a query transformation.
type Feature struct {
	ID    int
	Score float64
}

func (f Feature) Set(score float64) Feature {
	f.Score = score
	return f
}

const (
	// Context features.
	depthFeature         = iota
	clauseTypeFeature
	childrenCountFeature

	// Transformation-based features.
	logicalReplacementTypeFeature
	adjacencyChangeFeature
	adjacencyDistanceFeature
	meshDepthFeature
	restrictionTypeFeature

	// Pre-QPP-based features
	avgIDFFeature
	sumIDFFeature
	maxIDFFeature
	stdDevIDFFeature
	avgICTFFeature
)

func NewFeature(id int, score float64) Feature {
	return Feature{id, score}
}

// Features is the group of features used to learn or predict a score.
type Features []Feature

func (ff Features) Len() int           { return len(ff) }
func (ff Features) Swap(i, j int)      { ff[i], ff[j] = ff[j], ff[i] }
func (ff Features) Less(i, j int) bool { return ff[i].ID < ff[j].ID }

// LearntFeature contains the features that were used to produce a particular score.
type LearntFeature struct {
	Features
	Score float64
}

// TransformedQuery is the current most query in the query chain.
type TransformedQuery struct {
	QueryChain    []cqr.CommonQueryRepresentation
	PipelineQuery groove.PipelineQuery
}

// CandidateQuery is a possible transformation a query can take.
type CandidateQuery struct {
	Features
	Query cqr.CommonQueryRepresentation
}

func ContextFeatures(context TransformationContext) Features {
	return Features{
		NewFeature(depthFeature, context.Depth),
		NewFeature(clauseTypeFeature, context.ClauseType),
		NewFeature(childrenCountFeature, context.ChildrenCount),
	}
}

// QPPFeatures computes query performance predictor features for a query.
func QPPFeatures(query cqr.CommonQueryRepresentation, ss stats.StatisticsSource, me analysis.MeasurementExecutor) (Features, error) {
	gq := groove.NewPipelineQuery("qpp", 0, query)
	features := []int{avgIDFFeature, sumIDFFeature, maxIDFFeature, stdDevIDFFeature, avgICTFFeature}
	m, err := me.Execute(gq, ss, preqpp.AvgIDF, preqpp.SumIDF, preqpp.MaxIDF, preqpp.StdDevIDF, preqpp.AvgICTF)
	if err != nil {
		return nil, err
	}

	ff := make(Features, len(features))
	for i, feature := range features {
		ff[i] = NewFeature(feature, m[i])
	}

	return ff, nil
}

// String returns the string of a Feature family.
func (ff Features) String() string {
	var s string
	for _, f := range ff {
		s += fmt.Sprintf("%v:%v ", f.ID, f.Score)
	}
	return s
}

// WriteLibSVM writes a LIBSVM compatible line to a writer.
func (lf LearntFeature) WriteLibSVM(writer io.Writer, comment ...interface{}) (int, error) {
	sort.Sort(lf.Features)
	line := fmt.Sprintf("%v", lf.Score)
	for _, f := range lf.Features {
		line += fmt.Sprintf(" %v:%v", f.ID, f.Score)
	}
	if len(comment) > 0 {
		line += " #"
		for _, c := range comment {
			line += fmt.Sprintf(" %v", c)
		}
	}

	return writer.Write([]byte(line + "\n"))
}

// WriteLibSVMRank writes a LIBSVM^rank compatible line to a writer.
func (lf LearntFeature) WriteLibSVMRank(writer io.Writer, topic int64, comment string) (int, error) {
	sort.Sort(lf.Features)
	line := fmt.Sprintf("%v qid:%v", lf.Score, topic)
	for _, f := range lf.Features {
		b := f.ID
		line += fmt.Sprintf(" %v:%v", b+1, f.Score)
	}
	line += " # " + comment

	return writer.Write([]byte(line + "\n"))
}

// AverageScore compute the average Feature score for a group of features.
func (ff Features) AverageScore() float64 {
	if len(ff) == 0 {
		return 0
	}

	totalScore := 0.0
	for _, f := range ff {
		totalScore += f.Score
	}

	if totalScore == 0 {
		return 0
	}

	return totalScore / float64(len(ff))
}

func NewLearntFeature(score float64, features Features) LearntFeature {
	return LearntFeature{
		features,
		score,
	}
}

// Append adds the most recent query transformation to the chain and updates the current query.
func (t TransformedQuery) Append(query groove.PipelineQuery) TransformedQuery {
	t.QueryChain = append(t.QueryChain, t.PipelineQuery.Query)
	t.PipelineQuery = query
	return t
}

// NewTransformedQuery creates a new transformed query.
func NewTransformedQuery(query groove.PipelineQuery, chain ...cqr.CommonQueryRepresentation) TransformedQuery {
	return TransformedQuery{
		QueryChain:    chain,
		PipelineQuery: query,
	}
}

// NewCandidateQuery creates a new candidate query.
func NewCandidateQuery(query cqr.CommonQueryRepresentation, ff Features) CandidateQuery {
	return CandidateQuery{
		Features: ff,
		Query:    query,
	}
}
