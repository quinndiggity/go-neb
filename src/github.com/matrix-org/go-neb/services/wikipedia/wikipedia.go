// Package wikipedia implements a Service which adds !commands for Wikipedia search.
package wikipedia

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/jaytaylor/html2text"
	"github.com/matrix-org/go-neb/types"
	"github.com/matrix-org/gomatrix"
)

// ServiceType of the Wikipedia service
const ServiceType = "wikipedia"
const maxExtractLength = 1024 // Max length of extract string in bytes

var httpClient = &http.Client{}

type wikipediaSearchResults struct {
	Query wikipediaQuery `json:"query"`
}

type wikipediaQuery struct {
	Pages map[string]wikipediaPage `json:"pages"`
}

type wikipediaPage struct {
	PageID    int64  `json:"pageid"`
	NS        int    `json:"ns"`
	Title     string `json:"title"`
	Touched   string `json:"touched"`
	LastRevID int64  `json:"lastrevid"`
	Extract   string `json:"extract"`
}

// Service contains the Config fields for the Wikipedia service.
type Service struct {
	types.DefaultService
}

// Commands supported:
//    !wikipedia some_search_query_without_quotes
// Responds with a suitable article extract and link to the referenced page into the same room as the command.
func (s *Service) Commands(client *gomatrix.Client) []types.Command {
	return []types.Command{
		types.Command{
			Path: []string{"wikipedia"},
			Command: func(roomID, userID string, args []string) (interface{}, error) {
				return s.cmdWikipediaSearch(client, roomID, userID, args)
			},
		},
	}
}

// usageMessage returns a matrix TextMessage representation of the service usage
func usageMessage() *gomatrix.TextMessage {
	return &gomatrix.TextMessage{"m.notice",
		`Usage: !wikipedia search_text`}
}

func (s *Service) cmdWikipediaSearch(client *gomatrix.Client, roomID, userID string, args []string) (interface{}, error) {

	if len(args) < 1 {
		return usageMessage(), nil
	}

	// Get the query text to search for.
	querySentence := strings.Join(args, " ")

	searchResultPage, err := s.text2Wikipedia(querySentence)

	if err != nil {
		return nil, err
	}

	if searchResultPage.Extract == "" {
		return gomatrix.TextMessage{
			MsgType: "m.notice",
			Body:    "No results found!",
		}, nil
	}

	extractText, err := html2text.FromString(searchResultPage.Extract)
	if err != nil {
		return gomatrix.TextMessage{
			MsgType: "m.notice",
			Body:    "Failed to convert extract to plain text - " + err.Error(),
		}, nil
	}

	// Truncate the extract text, if necessary
	if len(extractText) > maxExtractLength {
		extractText = extractText[:maxExtractLength] + "..."
	}

	// Add a link to the bottom of the extract
	extractText += fmt.Sprintf("\nhttp://en.wikipedia.org/?curid=%d", searchResultPage.PageID)

	return gomatrix.TextMessage{
		MsgType: "m.notice",
		Body:    extractText,
	}, nil
}

// text2Wikipedia returns a Wikipedia article summary
func (s *Service) text2Wikipedia(query string) (*wikipediaPage, error) {
	log.Info("Searching Wikipedia for: ", query)

	u, err := url.Parse("https://en.wikipedia.org/w/api.php")
	if err != nil {
		return nil, err
	}

	// Example query - https://en.wikipedia.org/w/api.php?action=query&prop=extracts&format=json&exintro=&titles=RMS+Titanic
	q := u.Query()
	q.Set("action", "query")  // Action - query for articles
	q.Set("prop", "extracts") // Return article extracts
	q.Set("format", "json")
	// q.Set("exintro", "")
	q.Set("titles", query) // Text to search for

	u.RawQuery = q.Encode()
	// log.Info("Request URL: ", u)

	res, err := httpClient.Get(u.String())
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Request error: %d, %s", res.StatusCode, response2String(res))
	}
	var searchResults wikipediaSearchResults

	// log.Info(response2String(res))
	if err := json.NewDecoder(res.Body).Decode(&searchResults); err != nil {
		return nil, fmt.Errorf("ERROR - %s", err.Error())
	} else if len(searchResults.Query.Pages) < 1 {
		return nil, fmt.Errorf("No articles found")
	}

	// Return only the first search result with an extract
	for _, page := range searchResults.Query.Pages {
		if page.Extract != "" {
			return &page, nil
		}
	}

	return nil, fmt.Errorf("No articles with extracts found")
}

// response2String returns a string representation of an HTTP response body
func response2String(res *http.Response) string {
	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "Failed to decode response body"
	}
	str := string(bs)
	return str
}

// Initialise the service
func init() {
	types.RegisterService(func(serviceID, serviceUserID, webhookEndpointURL string) types.Service {
		return &Service{
			DefaultService: types.NewDefaultService(serviceID, serviceUserID, ServiceType),
		}
	})
}
