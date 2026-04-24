# mysql bootstrap scripts

SQL scripts that create the MySQL account datastorectl uses to
manage a cluster. Two variants ship: read-write for plan/apply
workflows, read-only for plan-only drift detection.

| Script | User | Grants | Use case |
|--------|------|--------|----------|
| `bootstrap-readwrite.sql` | `datastorectl@%` | `ALL PRIVILEGES WITH GRANT OPTION` globally + `SELECT` on mysql schema tables | Full plan/apply runs; the default operational account |
| `bootstrap-readonly.sql` | `datastorectl-ro@%` | `SELECT` only on mysql schema tables | CI drift detection; reviewer-level access; plan-only |

Both scripts enforce `REQUIRE SSL` on the account — plaintext auth
for a management user is not supported.

## Why the read-write user needs `ALL PRIVILEGES WITH GRANT OPTION`

The write-side grant matches what Puppet's `mysql` module does for
the same reason: a tool that manages grants must itself hold every
privilege it may pass on. Scoping down the management account below
`ALL PRIVILEGES` would mean datastorectl couldn't grant some
privileges downstream — the tool would fail at apply time with
confusing partial-grant errors.

This is a category constraint, not a datastorectl choice. Every
declarative grant-management tool hits it.

## Running the scripts

Replace `<REPLACE_ME>` in the chosen script with a strong password,
then apply against the cluster as the root (or equivalent) user:

```
mysql -u root -p < providers/mysql/bootstrap/bootstrap-readwrite.sql
```

After the read-write script runs, use the new credentials in a DCL
context block:

```hcl
context "prod" {
  provider = mysql
  version  = "8.4"
  endpoint = "prod.example.com:3306"
  auth     = "password"
  username = "datastorectl"
  password = secret("env", "DATASTORECTL_MYSQL_PASSWORD")
  tls      = "required"
}
```

For the read-only variant, substitute `datastorectl-ro` and its
password. Running `datastorectl apply` under the read-only account
will fail at the first write with a clear MySQL permission error —
switch to the read-write account for apply workflows.

## On AWS RDS

The scripts work against self-hosted MySQL and RDS. For RDS with IAM
authentication, replace the `IDENTIFIED WITH caching_sha2_password`
clause with:

```sql
CREATE USER 'datastorectl'@'%'
  IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS'
  REQUIRE SSL;
```

See `providers/mysql/README.md` for the full IAM-authentication
setup including the required IAM policy.
