package analyzer

import (
	"github.com/VirtusLab/jira-stats/analyzer/domain"
	"github.com/ztrue/tracerr"
	"log"
	"strconv"
	"strings"
	"time"
)

// Generates CSV contents from DB
func GetCsv(startDate time.Time, endDate time.Time) (*domain.CsvContents, error) {
	log.Printf("Fetching tickets for dev time between (%s, %s)\n", startDate.Format(time.RFC3339), endDate.Format(time.RFC3339))

	ticketsWithDevBefore, err := fetchTicketsWithDevStartTimeBefore(startDate, endDate)
	if err != nil {
		return &domain.CsvContents{}, tracerr.Wrap(err)
	}
	log.Printf("Fetched %d tickets...\n", len(ticketsWithDevBefore))

	rows := make([]domain.CsvRow, 0)

	for _, ticket := range ticketsWithDevBefore {
		rows = append(rows, domain.CsvRow{
			Entries: []string{
				ticket.Key, ticket.Type, csvEscape(ticket.Title), ticket.Project(),
				strconv.FormatFloat(domain.CalculateDevDays(ticket, startDate, endDate), 'f', 2, 64),
			},
		})
	}

	return &domain.CsvContents{
		Header: []string{"Key", "Type", "Summary", "Project", "Dev Time (days)"},
		Rows:   rows,
	}, nil
}

func csvEscape(str string) string {
	return strings.ReplaceAll(str, ",", " ")
}
