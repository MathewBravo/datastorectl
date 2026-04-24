package mysql

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

// rdsIAMPingSuggestion picks a diagnostic suggestion for a failed RDS
// IAM connection check. A MySQL 1045 (access denied) under rds_iam
// auth almost always means the signing principal is missing
// rds-db:connect on the target DB user, or the server-side user was
// not created with AWSAuthenticationPlugin. Other errors get the
// generic endpoint/TLS hint.
func rdsIAMPingSuggestion(err error) string {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1045 {
		return `for rds_iam auth, Error 1045 almost always means the signing principal is missing rds-db:connect on this DB user, or the server-side user is not IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS'; verify the IAM policy resource ARN matches arn:aws:rds-db:<region>:<account>:dbuser:<cluster-resource-id>/<username>`
	}
	return "verify the RDS endpoint is reachable, the IAM user is enabled on the DB, and TLS is accepted"
}
