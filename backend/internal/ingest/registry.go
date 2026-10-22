package ingest

import (
	"context"
	"log/slog"
	"sort"
	"strings"
)

// Registry holds every enabled source by name.
type Registry struct {
	sources map[string]Source
	log     *slog.Logger
}

// BuildRegistry constructs every enabled connector from config.
func BuildRegistry(cfg *Config, client *Client, log *slog.Logger) *Registry {
	r := &Registry{sources: map[string]Source{}, log: log}

	ats := cfg.Sources.ATS
	if ats.Greenhouse.Enabled {
		for _, slug := range ats.Greenhouse.Slugs {
			s := &greenhouseSource{slug: slug, client: client, base: defaultGreenhouseBase, interval: ats.Greenhouse.IntervalSeconds}
			r.add(s.Name(), s)
		}
	}
	if ats.Lever.Enabled {
		for _, slug := range ats.Lever.Slugs {
			s := &leverSource{slug: slug, client: client, base: defaultLeverBase, interval: ats.Lever.IntervalSeconds}
			r.add(s.Name(), s)
		}
	}
	if ats.Ashby.Enabled {
		for _, slug := range ats.Ashby.Slugs {
			s := &ashbySource{slug: slug, client: client, base: defaultAshbyBase, interval: ats.Ashby.IntervalSeconds}
			r.add(s.Name(), s)
		}
	}
	if cfg.Sources.RemoteOK.Enabled {
		s := &remoteOKSource{client: client, base: defaultRemoteOKBase, interval: cfg.Sources.RemoteOK.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.Remotive.Enabled {
		s := &remotiveSource{client: client, base: defaultRemotiveBase, searches: cfg.Sources.Remotive.Searches, interval: cfg.Sources.Remotive.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.Himalayas.Enabled {
		s := &himalayasSource{client: client, base: defaultHimalayasBase, maxPages: cfg.Sources.Himalayas.MaxPages, interval: cfg.Sources.Himalayas.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.Jobicy.Enabled {
		s := &jobicySource{client: client, base: defaultJobicyBase, count: cfg.Sources.Jobicy.Count, interval: cfg.Sources.Jobicy.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.WeWorkRemotely.Enabled {
		s := &wwrSource{client: client, feeds: defaultWWRFeeds, interval: cfg.Sources.WeWorkRemotely.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.Arbeitnow.Enabled {
		s := &arbeitnowSource{client: client, base: defaultArbeitnowBase, maxPages: cfg.Sources.Arbeitnow.MaxPages, interval: cfg.Sources.Arbeitnow.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.WorkingNomads.Enabled {
		s := &workingNomadsSource{client: client, base: defaultWorkingNomadsBase, interval: cfg.Sources.WorkingNomads.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.PythonOrg.Enabled {
		s := &pythonOrgSource{client: client, base: defaultPythonOrgBase, interval: cfg.Sources.PythonOrg.IntervalSeconds}
		r.add(s.Name(), s)
	}
	if cfg.Sources.HackerNews.Enabled {
		s := &hackerNewsSource{client: client, searchBase: defaultHNSearchBase, itemBase: defaultHNItemBase, monthsBack: cfg.Sources.HackerNews.MonthsBack, interval: cfg.Sources.HackerNews.IntervalSeconds}
		r.add(s.Name(), s)
	}
	return r
}

func (r *Registry) add(name string, s Source) {
	if _, exists := r.sources[name]; exists {
		r.log.Warn("duplicate source name ignored", "source", name)
		return
	}
	r.sources[name] = s
}

// All returns sources in stable name order.
func (r *Registry) All() []Source {
	names := make([]string, 0, len(r.sources))
	for n := range r.sources {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Source, 0, len(names))
	for _, n := range names {
		out = append(out, r.sources[n])
	}
	return out
}

// Get returns one source by name.
func (r *Registry) Get(name string) (Source, bool) {
	s, ok := r.sources[name]
	return s, ok
}

// Names lists source names in stable order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.sources))
	for n := range r.sources {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ATSBase names the three ATS sources whose slugs are probed at startup.
const (
	prefixGreenhouse = "greenhouse:"
	prefixLever      = "lever:"
	prefixAshby      = "ashby:"
)

// ValidateSlugs probes every ATS slug once. Dead slugs are removed from the
// registry for this process and logged together in a single line. Transient
// failures keep the slug so a network blip does not drop a live board.
func (r *Registry) ValidateSlugs(ctx context.Context) {
	var dead []string
	for name, s := range r.sources {
		v, ok := s.(validatable)
		if !ok {
			continue
		}
		if err := v.Validate(ctx); err != nil {
			if isDeadSlug(err) {
				dead = append(dead, name)
				delete(r.sources, name)
			} else {
				r.log.Warn("ats slug probe inconclusive, keeping source", "source", name, "error", err)
			}
		}
	}
	if len(dead) == 0 {
		return
	}
	sort.Strings(dead)
	r.log.Warn("dead ats slugs skipped", "count", len(dead), "slugs", strings.Join(dead, ", "))
}

func isDeadSlug(err error) bool {
	return statusOf(err) == 404
}
