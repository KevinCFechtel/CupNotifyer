package update

import "strings"

type Kind string

const (
	KindVersion Kind = "version"
	KindDigest  Kind = "digest"
)

type Available struct {
	Reference      string
	Server         string
	URL            string
	Kind           Kind
	UpdateType     string
	CurrentVersion string
	NewVersion     string
	NewTag         string
	LocalDigests   []string
	RemoteDigest   string
}

func (u Available) ImageID() string {
	return u.Server + "|" + u.Reference
}

func (u Available) UpdateID() string {
	switch u.Kind {
	case KindVersion:
		return string(u.Kind) + ":" + u.NewTag
	case KindDigest:
		return string(u.Kind) + ":" + u.RemoteDigest
	default:
		return string(u.Kind) + ":" + strings.TrimSpace(u.NewTag+u.RemoteDigest)
	}
}
