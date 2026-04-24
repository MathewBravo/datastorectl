GRANT PROCESS, REPLICATION CLIENT ON *.* TO `basic_user`@`%`
GRANT SELECT, INSERT, UPDATE, DELETE ON `appdb`.* TO `basic_user`@`%`
GRANT ALL PRIVILEGES ON `appdb`.`users` TO `basic_user`@`%` WITH GRANT OPTION
