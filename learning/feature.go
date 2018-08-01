package learning

import (
	"fmt"
	"github.com/hscells/cqr"
	"github.com/hscells/groove"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/groove/stats"
	"github.com/xtgo/set"
	"io"
	"sort"
	"strings"
	"github.com/go-errors/errors"
	"github.com/hscells/groove/analysis/preqpp"
)

// Feature is some value that is applicable to a query transformation.
type Feature struct {
	ID    int
	Score float64
}

// Set sets the `score` of the feature.
func (f Feature) Set(score float64) Feature {
	f.Score = score
	return f
}

type deltaFeatures map[int]float64

const (
	// Context features.
	nilFeature           = iota
	DepthFeature
	ClauseTypeFeature     // This isn't the operator type, it's the type of the clause (keyword query/Boolean query).
	ChildrenCountFeature

	// Transformation-based features.
	TransformationTypeFeature
	LogicalReplacementTypeFeature
	AdjacencyReplacementFeature
	AdjacencyDistanceFeature
	MeshDepthFeature
	MeshParentFeature
	RestrictionTypeFeature
	ClauseRemovalFeature
	Cui2vecExpansionFeature
	Cui2vecNumExpansionsFeature

	// Keyword specific features.
	IsExplodedFeature
	IsTruncatedFeature
	NumFieldsFeature

	// Boolean specific features.
	OperatorTypeFeature

	// Measurement features.
	measurementFeatures

	// Chain of transformations !!THIS MUST BE THE LAST FEATURE IN THE LIST!!
	chainFeatures
)

// MeasurementFeatureKeys contains a mapping of applicable measurement to a feature.
var MeasurementFeatureKeys = map[string]int{
	analysis.BooleanFields.Name():        measurementFeatures,
	analysis.BooleanClauses.Name():       measurementFeatures + 1,
	analysis.BooleanKeywords.Name():      measurementFeatures + 2,
	analysis.BooleanTruncated.Name():     measurementFeatures + 3,
	preqpp.RetrievalSize.Name():          measurementFeatures + 4,
	preqpp.QueryScope.Name():             measurementFeatures + 5,
	analysis.MeshKeywordCount.Name():     measurementFeatures + 6,
	analysis.MeshExplodedCount.Name():    measurementFeatures + 7,
	analysis.MeshNonExplodedCount.Name(): measurementFeatures + 8,
	analysis.MeshAvgDepth.Name():         measurementFeatures + 9,
	analysis.MeshMaxDepth.Name():         measurementFeatures + 10,
}

// Chain of transformations !!THIS MUST BE THE LAST FEATURE IN THE LIST!!
var ChainFeatures = chainFeatures + len(MeasurementFeatureKeys)*2

// NewFeature creates a new feature with the specified ID and `score`.
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
	Scores  []float64
	Topic   string
	Comment string
}

// TransformedQuery is the current most query in the query chain.
type TransformedQuery struct {
	QueryChain    []cqr.CommonQueryRepresentation
	PipelineQuery groove.PipelineQuery
}

// CandidateQuery is a possible transformation a query can take.
type CandidateQuery struct {
	Features
	Query            cqr.CommonQueryRepresentation
	TransformationID int
	Chain            []CandidateQuery
}

func keywordFeatures(q cqr.Keyword) Features {
	var features Features

	// Exploded.
	// 0 - not applicable.
	// 1 - not exploded.
	// 2 - exploded.
	explodedFeature := NewFeature(IsExplodedFeature, 0)
	if _, ok := q.Options["exploded"]; ok {
		if exploded, ok := q.Options["exploded"].(bool); ok && exploded {
			explodedFeature.Score = 2
		} else {
			explodedFeature.Score = 1
		}
	}

	// Truncation. Same feature values as exploded.
	truncatedFeature := NewFeature(IsTruncatedFeature, 0)
	if _, ok := q.Options["truncated"]; ok {
		if truncated, ok := q.Options["truncated"].(bool); ok && truncated {
			truncatedFeature.Score = 2
		} else {
			truncatedFeature.Score = 1
		}
	}

	// Number of fields the query has.
	numFields := NewFeature(NumFieldsFeature, float64(len(q.Fields)))

	features = append(features, explodedFeature, truncatedFeature, numFields)
	return features
}

func booleanFeatures(q cqr.BooleanQuery) Features {
	var features Features

	// Operator type feature.
	// 0 - n/a
	// 1 - or
	// 2 - and
	// 3 - not
	// 4 - adj
	operatorType := NewFeature(OperatorTypeFeature, 0)
	switch q.Operator {
	case "or", "OR":
		operatorType.Score = 1
	case "and", "AND":
		operatorType.Score = 2
	case "not", "NOT":
		operatorType.Score = 3
	default:
		if strings.Contains(q.Operator, "adj") {
			operatorType.Score = 4
		}

	}
	features = append(features, operatorType)

	return features
}

func contextFeatures(context TransformationContext) Features {
	var features Features

	return append(features,
		NewFeature(DepthFeature, context.Depth),
		NewFeature(ClauseTypeFeature, context.ClauseType),
		NewFeature(ChildrenCountFeature, context.ChildrenCount))
}

// QPPFeatures computes query performance predictor features for a query.
func deltas(query cqr.CommonQueryRepresentation, ss stats.StatisticsSource, measurements []analysis.Measurement, me analysis.MeasurementExecutor) (deltaFeatures, error) {
	deltas := make(deltaFeatures)

	gq := groove.NewPipelineQuery("qpp", "test", query)
	m, err := me.Execute(gq, ss, measurements...)
	if err != nil {
		return nil, err
	}
	for i, measurement := range measurements {
		if v, ok := MeasurementFeatureKeys[measurement.Name()]; ok {
			deltas[v] = m[i]
		} else {
			return nil, errors.New(fmt.Sprintf("%s is not registed as a feature in MeasurementFeatureKeys", measurement.Name()))
		}
	}

	return deltas, nil
}

func calcDelta(feature int, score float64, features deltaFeatures) float64 {
	if y, ok := features[feature]; ok {
		return score - y
	}
	return 0
}

func computeDeltas(preTransformation deltaFeatures, postTransformation deltaFeatures) Features {
	var features Features
	for feature, x := range preTransformation {
		deltaFeature := feature + len(MeasurementFeatureKeys)
		features = append(features, NewFeature(deltaFeature, calcDelta(feature, x, postTransformation)))
	}
	return features
}

// transformationFeature is a feature representing the previous feature that was applied to a query.
func transformationFeature(transformer Transformer) Feature {
	transformationType := NewFeature(TransformationTypeFeature, 0)
	switch transformer.(type) {
	case logicalOperatorReplacement:
		transformationType.Score = 1
	case *adjacencyRange:
		transformationType.Score = 2
	case meshExplosion:
		transformationType.Score = 3
	case fieldRestrictions:
		transformationType.Score = 4
	case adjacencyReplacement:
		transformationType.Score = 5
	}
	return transformationType
}

// String returns the string of a Feature family.
func (ff Features) String() string {
	sort.Sort(ff)
	size := set.Uniq(ff)
	tmp := ff[:size]
	s := "0 "
	for _, f := range tmp {
		s += fmt.Sprintf("%v:%v ", f.ID, f.Score)
	}
	return s
}

// WriteLibSVM writes a LIBSVM compatible line to a writer.
func (lf LearntFeature) WriteLibSVM(writer io.Writer, comment ...interface{}) (int, error) {
	sort.Sort(lf.Features)
	size := set.Uniq(lf.Features)
	ff := lf.Features[:size]
	line := fmt.Sprintf("%v", lf.Scores[0])
	for _, f := range ff {
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
func (lf LearntFeature) WriteLibSVMRank(writer io.Writer) (int, error) {
	sort.Sort(lf.Features)
	size := set.Uniq(lf.Features)
	ff := lf.Features[:size]
	line := fmt.Sprintf("%v qid:%v", lf.Scores[0], lf.Topic)
	for _, f := range ff {
		line += fmt.Sprintf(" %v:%v", f.ID, f.Score)
	}
	line += " # " + lf.Comment

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

// NewLearntFeature creates a new learnt feature with a score and a set of features.
func NewLearntFeature(features Features) LearntFeature {
	return LearntFeature{
		Features: features,
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
		Features:         ff,
		Query:            query,
		TransformationID: -1,
	}
}

// SetTransformationID sets the transformation id to the candidate query.
func (c CandidateQuery) SetTransformationID(id int) CandidateQuery {
	c.TransformationID = id
	return c
}

// Append adds the previous query to the chain of transformations so far so we can keep track of which transformations
// have been applied up until this point, and for features about the query.
func (c CandidateQuery) Append(query CandidateQuery) CandidateQuery {
	c.Chain = query.Chain

	// Chain features is the minimum possible index for these features.
	idx := ChainFeatures
	for i, candidate := range c.Chain {
		c.Features = append(c.Features, NewFeature(idx+i, float64(candidate.TransformationID)))
	}

	c.Chain = append(c.Chain, query)

	return c
}