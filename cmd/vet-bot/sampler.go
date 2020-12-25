package main

import (
	"encoding/csv"
	"errors"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"
)

// RepositorySampler maintains the state of unvisited repositories and provides a mechanism
// for visiting them at random.
type RepositorySampler struct {
	m             sync.Mutex
	unvisited     []Repository
	visitedFile   *MutexWriter
	visitedWriter *csv.Writer
}

// NewRepositorySampler initializes the repository sampler by opening two CSV files. The first file
// consists of the list of repositories from which to sample. The second file -- which will be created
// if it doesn't already exist -- stores a sublist of the repositories which have already been visited.
func NewRepositorySampler(allFile string, visitedFile string) (*RepositorySampler, error) {
	repos, err := readRepositoryList(allFile)
	if err != nil {
		return nil, err
	}
	repoList := make([]Repository, 0, len(repos))
	for repo := range repos {
		repoList = append(repoList, repo)
	}
	log.Printf("repository list loaded from %s", allFile)
	if _, err := os.Stat(visitedFile); err == nil {
		visitedRepos, err := readRepositoryList(visitedFile)
		if err != nil {
			return nil, err
		}
		for repo := range visitedRepos {
			delete(repos, repo)
		}
	}

	visitedWriter, err := os.OpenFile(visitedFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	log.Printf("visited list loaded from %s", visitedFile)
	mw := NewMutexWriter(visitedWriter)
	return &RepositorySampler{
		unvisited:     repoList,
		visitedFile:   &mw,
		visitedWriter: csv.NewWriter(&mw),
	}, nil
}

// Sample is used to sample a repository from the list of repositories managed by this sampler. A handler function
// is passed which receives the repository sampled from the list. If the handler returns nil, the sampled repository
// is removed from the list and is not visited again. If the handler returns an error, the sampled repository is
// not removed from the list and may be visited again. Sample only returns an error itself if no further samples should
// be made.
func (rs *RepositorySampler) Sample(handler func(Repository) error) error {
	if len(rs.unvisited) == 0 {
		return errors.New("no unvisited repositories left to sample")
	}
	repo := rs.sampleAndReturn()
	err := handler(repo)

	if err != nil {
		rs.m.Lock()
		defer rs.m.Unlock()
		rs.unvisited = append(rs.unvisited, repo)
		log.Printf("repo %s/%s will be tried again despite error: %v", repo.Owner, repo.Repo, err)
		return nil
	}

	err = rs.visitedWriter.Write([]string{repo.Owner, repo.Repo})
	rs.visitedWriter.Flush()
	if err != nil {
		log.Fatalf("could not write to output file: %v", err)
		return err
	}
	return nil
}

func (rs *RepositorySampler) sampleAndReturn() Repository {
	rs.m.Lock()
	defer rs.m.Unlock()
	idx := rand.Intn(len(rs.unvisited))
	repo := rs.unvisited[idx]
	rs.unvisited[idx] = rs.unvisited[len(rs.unvisited)-1]
	rs.unvisited = rs.unvisited[:len(rs.unvisited)-1]
	return repo
}

// Close closes the file.
func (rs *RepositorySampler) Close() error {
	return rs.visitedFile.Close()
}

// readRepositoryList retrieves a set of repositories which have already been read from.
func readRepositoryList(filename string) (map[Repository]struct{}, error) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	result := make(map[Repository]struct{})
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) != 2 {
			log.Printf("malformed line in repository list %s", filename)
			continue
		}
		result[Repository{
			Owner: record[0],
			Repo:  record[1],
		}] = struct{}{}
	}
	return result, nil
}

// MutexWriter wraps an io.WriteCloser with a sync.Mutex.
type MutexWriter struct { // TODO: this might be a bit much; we already have a Mutex in RepositorySampler
	m sync.Mutex
	w io.WriteCloser
}

// NewMutexWriter wraps an io.WriteCloser with a sync.Mutex.
func NewMutexWriter(w io.WriteCloser) MutexWriter {
	return MutexWriter{w: w}
}

func (mw *MutexWriter) Write(b []byte) (int, error) {
	mw.m.Lock()
	defer mw.m.Unlock()
	return mw.w.Write(b)
}

// Close closes the underlying WriteCloser.
func (mw *MutexWriter) Close() error {
	mw.m.Lock()
	defer mw.m.Unlock()
	return mw.w.Close()
}
