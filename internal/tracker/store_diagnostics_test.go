package tracker

import (
	"errors"
	"github.com/go-sql-driver/mysql"
	"net"
	"strings"
	"testing"
)

func TestConnectionDiagnosticsNeverExposeUnderlyingMessages(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{&mysql.MySQLError{Number: 1045, Message: "sensitive-test-value"}, "rejected authentication"},
		{&mysql.MySQLError{Number: 1049, Message: "sensitive-test-value"}, "database not found"},
		{&net.DNSError{Err: "sensitive-test-value", Name: "private-host"}, "hostname could not be resolved"},
		{errors.New("sensitive-test-value"), "before a database session"},
	} {
		got := connectionFailure(test.err).Error()
		if !strings.Contains(got, test.want) || strings.Contains(got, "sensitive-test-value") || strings.Contains(got, "private-host") {
			t.Fatalf("unsafe or unhelpful diagnostic: %s", got)
		}
	}
}
