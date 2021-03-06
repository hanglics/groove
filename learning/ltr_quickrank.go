package learning

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"github.com/hscells/groove/stats"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
)

type QuickRankQueryCandidateSelector struct {
	// The path to the binary file for execution.
	binary string
	// Maximum depth allowed to generate queries.
	depth        int
	currentDepth int
	// Statistics source.
	s stats.StatisticsSource
	// Command-line arguments for configuration.
	arguments map[string]interface{}
}

func makeArguments(a map[string]interface{}) []string {
	// Load the arguments from the map.
	args := make([]string, len(a)*2)
	i := 0
	for k, v := range a {
		args[i] = fmt.Sprintf("--%s", k)
		args[i+1] = fmt.Sprintf("%v", v)
		i += 2
	}
	return args
}

func (qr QuickRankQueryCandidateSelector) Select(query CandidateQuery, transformations []CandidateQuery) (CandidateQuery, QueryChainCandidateSelector, error) {
	args := makeArguments(qr.arguments)

	fname := uuid.New().String()
	args = append(args, "--test", fname)
	defer os.Remove(fname)

	// Create a temporary file to contain the Features for testing.
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Write the Features of the variation to temporary file.
	for _, applied := range transformations {
		_, err := f.WriteString(fmt.Sprintf("0 qid:%s %s\n", query.Topic, applied.Features.String()))
		if err != nil {
			return query, qr, err
		}
	}

	// Configure the command.
	cmd := exec.Command(qr.binary, args...)

	// Open channels to stdout and stderr.
	r, err := cmd.StdoutPipe()
	if err != nil {
		return query, qr, err
	}
	defer r.Close()

	e, err := cmd.StderrPipe()
	if err != nil {
		return query, qr, err
	}
	defer e.Close()

	// Start the command.
	cmd.Start()

	// Output the stdout pipe.
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			log.Println(s.Text())
		}
		return
	}()

	// Output the stderr pipe.
	go func() {
		s := bufio.NewScanner(e)
		for s.Scan() {
			log.Println(s.Text())
		}
		return
	}()

	// Wait for the command to finish.
	if err := cmd.Wait(); err != nil {
		return query, qr, err
	}

	// Grab the top-most ranked query from the candidates.
	candidate, err := getRanking(qr.arguments["scores"].(string), transformations)
	if err != nil {
		return query, qr, err
	}
	defer os.Remove(qr.arguments["scores"].(string))

	// Totally remove the file.
	f.Truncate(0)
	f.Seek(0, 0)

	ret, err := qr.s.RetrievalSize(query.Query)
	if err != nil {
		return CandidateQuery{}, nil, err
	}
	if ret == 0 {
		log.Println("stopping early")
		qr.currentDepth = qr.depth
		return query, qr, nil
	}
	log.Printf("numret: %f\n", ret)

	qr.currentDepth++

	if query.Query.String() == candidate.String() {
		qr.currentDepth = math.MaxInt32
	}

	return candidate, qr, nil
}

func (qr QuickRankQueryCandidateSelector) Train(lfs []LearntFeature) ([]byte, error) {
	args := makeArguments(qr.arguments)

	// Configure the command.
	cmd := exec.Command(qr.binary, args...)

	// Open channels to stdout and stderr.
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	e, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	defer e.Close()

	// Start the command.
	cmd.Start()

	// Output the stdout pipe.
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			log.Println(s.Text())
		}
		return
	}()

	// Output the stderr pipe.
	go func() {
		s := bufio.NewScanner(e)
		for s.Scan() {
			log.Println(s.Text())
		}
		return
	}()

	// Wait for the command to finish.
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return nil, nil
}

func (QuickRankQueryCandidateSelector) Output(lf LearntFeature, w io.Writer) error {
	_, err := lf.WriteLibSVMRank(w)
	return err
}

func (qr QuickRankQueryCandidateSelector) StoppingCriteria() bool {
	return qr.currentDepth >= qr.depth
}

func QuickRankCandidateSelectorMaxDepth(d int) func(c *QuickRankQueryCandidateSelector) {
	return func(c *QuickRankQueryCandidateSelector) {
		c.depth = d
	}
}

func QuickRankCandidateSelectorStatisticsSource(s stats.StatisticsSource) func(c *QuickRankQueryCandidateSelector) {
	return func(c *QuickRankQueryCandidateSelector) {
		c.s = s
	}
}

func NewQuickRankQueryCandidateSelector(binary string, arguments map[string]interface{}, args ...func(c *QuickRankQueryCandidateSelector)) QuickRankQueryCandidateSelector {
	q := &QuickRankQueryCandidateSelector{
		binary:       binary,
		arguments:    arguments,
		depth:        5,
		currentDepth: 0,
	}

	for _, arg := range args {
		arg(q)
	}

	fmt.Printf("created quick rank query candidate selector with depth %d\n", q.depth)
	return *q
}
