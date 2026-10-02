package lookup

import (
	"context"
	"sync"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

type WikiPage struct{ Title, URL string }
type WikiResult struct {
	Name      string
	Pages     []WikiPage
	Available bool
}

func QueryWiki(ctx context.Context, l i18n.Lang, q string) []WikiResult {
	results := make([]WikiResult, len(WikiSources))
	var wg sync.WaitGroup
	for i, source := range WikiSources {
		wg.Add(1)
		go func(i int, source WikiSource) {
			defer wg.Done()
			raw, available := SearchTitles(ctx, source, q, 24)
			titles := source.PickWikiTitles(l, raw, 4)
			display := DisplayTitles(ctx, source, titles)
			result := WikiResult{Name: source.Name, Available: available, Pages: make([]WikiPage, 0, len(titles))}
			for _, title := range titles {
				label := CleanDisplayTitle(display[title])
				if label == "" {
					label = title
				}
				result.Pages = append(result.Pages, WikiPage{Title: label, URL: source.PageURL(title)})
			}
			results[i] = result
		}(i, source)
	}
	wg.Wait()
	return results
}
