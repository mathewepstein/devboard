package state

import (
	"sync"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
)

type State struct {
	PolledAt time.Time  `json:"polledAt"`
	Errors   Errors     `json:"errors"`
	Budget   Budget     `json:"budget"`
	Repos    []Repo     `json:"repos"`
	Items    []Item     `json:"items"`
	Mentions []Mention  `json:"mentions"`
	Assigned []Assigned `json:"assignedNew"`
	Selected []Selected `json:"selectedNew"`
}

type Errors struct {
	GitHub string `json:"github,omitempty"`
	Jira   string `json:"jira,omitempty"`
}

type Budget struct {
	GitHub GitHubBudget `json:"github"`
}

type GitHubBudget struct {
	Cost      int       `json:"cost"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"resetAt"`
	Interval  string    `json:"interval"`
}

type Repo struct {
	Name   string   `json:"name"`
	GHRepo string   `json:"ghRepo"`
	Prefix string   `json:"prefix"`
	Live   *Version `json:"live"`
	Next   *Version `json:"next"`
	Queued []string `json:"queued"`
	PRs    []PR     `json:"prs"`
}

type Version struct {
	Name        string   `json:"name"`
	ReleaseDate string   `json:"releaseDate,omitempty"`
	Tickets     []Ticket `json:"tickets,omitempty"`
}

type Ticket struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
	Status   string `json:"status"`
	Category string `json:"category"`
	Assignee string `json:"assignee,omitempty"`
}

type PR struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Author         string    `json:"author"`
	Draft          bool      `json:"draft"`
	ReviewDecision string    `json:"reviewDecision"`
	Mergeable      string    `json:"mergeable"`
	Checks         string    `json:"checks"`
	HeadSHA        string    `json:"headSha"`
	HeadRefName    string    `json:"headRefName"`
	Requested      []string  `json:"requested"`
	Reviewers      []string  `json:"reviewers"`
	UpdatedAt      time.Time `json:"updatedAt"`
	TicketKeys     []string  `json:"ticketKeys"`
}

type Item struct {
	ID        string    `json:"id"`
	Rule      string    `json:"rule"`
	Detail    string    `json:"detail"`
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Mention struct {
	Key           string    `json:"key"`
	Summary       string    `json:"summary"`
	URL           string    `json:"url"`
	Status        string    `json:"status"`
	Updated       time.Time `json:"updated"`
	LastCommentBy string    `json:"lastCommentBy"`
}

type Assigned struct {
	Key        string    `json:"key"`
	Summary    string    `json:"summary"`
	URL        string    `json:"url"`
	Status     string    `json:"status"`
	Type       string    `json:"type"`
	AssignedAt time.Time `json:"assignedAt"`
}

type Selected struct {
	Key      string    `json:"key"`
	Summary  string    `json:"summary"`
	URL      string    `json:"url"`
	Assignee string    `json:"assignee,omitempty"`
	MovedAt  time.Time `json:"movedAt"`
}

// Store holds the last good State; swapped whole under a mutex.
type Store struct {
	mu sync.RWMutex
	s  State
}

func NewStore() *Store { return &Store{} }

func (st *Store) Get() State {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s
}

func (st *Store) Set(s State) {
	st.mu.Lock()
	st.s = s
	st.mu.Unlock()
}

// Empty State shaped by config, pre-first-poll.
func Seed(cfg *config.Config) State {
	repos := make([]Repo, len(cfg.Repos))
	for i, r := range cfg.Repos {
		repos[i] = Repo{Name: r.Name, GHRepo: r.Repo, Prefix: r.Prefix, Queued: []string{}, PRs: []PR{}}
	}
	return State{
		Repos:    repos,
		Items:    []Item{},
		Mentions: []Mention{},
		Assigned: []Assigned{},
		Selected: []Selected{},
	}
}
