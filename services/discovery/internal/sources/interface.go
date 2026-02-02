package sources

type VideoItem struct {
	URL      string
	Platform string
	Author   string
}

type Source interface {
	Name() string
	FetchRecent(hashtag string) ([]VideoItem, error)
}
