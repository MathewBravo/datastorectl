package mysql

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestRDSIAMPingSuggestion(t *testing.T) {
	t.Run("1045_points_at_rds_db_connect", func(t *testing.T) {
		got := rdsIAMPingSuggestion(&mysql.MySQLError{
			Number:  1045,
			Message: "Access denied for user 'datastorectl_iam'@'x.x.x.x' (using password: YES)",
		})
		for _, want := range []string{"rds-db:connect", "AWSAuthenticationPlugin"} {
			if !strings.Contains(got, want) {
				t.Errorf("suggestion missing %q\ngot: %s", want, got)
			}
		}
	})

	t.Run("wrapped_1045_also_matches", func(t *testing.T) {
		// The Go mysql driver may surface the 1045 wrapped via driver.ErrBadConn
		// retry logic or a fmt.Errorf chain — errors.As must unwrap.
		inner := &mysql.MySQLError{Number: 1045, Message: "Access denied"}
		wrapped := fmt.Errorf("opening mysql connection: %w", inner)
		got := rdsIAMPingSuggestion(wrapped)
		if !strings.Contains(got, "rds-db:connect") {
			t.Errorf("wrapped 1045 should still map to IAM suggestion; got: %s", got)
		}
	})

	t.Run("non_1045_mysql_error_uses_generic", func(t *testing.T) {
		// 2003 is "Can't connect to MySQL server" — real endpoint/TLS problem.
		got := rdsIAMPingSuggestion(&mysql.MySQLError{Number: 2003, Message: "Can't connect"})
		if strings.Contains(got, "rds-db:connect") {
			t.Errorf("non-1045 should not mention rds-db:connect; got: %s", got)
		}
		if !strings.Contains(got, "endpoint") {
			t.Errorf("generic suggestion should mention endpoint; got: %s", got)
		}
	})

	t.Run("non_mysql_error_uses_generic", func(t *testing.T) {
		got := rdsIAMPingSuggestion(errors.New("i/o timeout"))
		if strings.Contains(got, "rds-db:connect") {
			t.Errorf("non-MySQL error should not mention rds-db:connect; got: %s", got)
		}
	})
}
