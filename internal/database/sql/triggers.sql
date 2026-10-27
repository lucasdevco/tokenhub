CREATE TRIGGER records_insert AFTER INSERT ON records BEGIN
 INSERT INTO totals VALUES (new.source,new.state,new.method,new.capture_error,new.failed,1)
 ON CONFLICT(source,state,method,capture_error,failed) DO UPDATE SET n=n+1;
 INSERT INTO buckets VALUES (new.source,new.started/120000000000,new.state,new.method,new.capture_error,new.failed,1)
 ON CONFLICT(source,bucket,state,method,capture_error,failed) DO UPDATE SET n=n+1;
 END;
CREATE TRIGGER records_delete AFTER DELETE ON records BEGIN
 INSERT INTO totals VALUES (old.source,old.state,old.method,old.capture_error,old.failed,-1)
 ON CONFLICT(source,state,method,capture_error,failed) DO UPDATE SET n=n+-1;
 INSERT INTO buckets VALUES (old.source,old.started/120000000000,old.state,old.method,old.capture_error,old.failed,-1)
 ON CONFLICT(source,bucket,state,method,capture_error,failed) DO UPDATE SET n=n+-1;
 END;
CREATE TRIGGER records_update AFTER UPDATE OF started,state,method,capture_error,failed ON records
WHEN old.started<>new.started OR old.state<>new.state OR old.method<>new.method OR old.capture_error<>new.capture_error OR old.failed<>new.failed
BEGIN
 UPDATE totals SET n=n-1 WHERE source=old.source AND state=old.state AND method=old.method AND capture_error=old.capture_error AND failed=old.failed;
 UPDATE buckets SET n=n-1 WHERE source=old.source AND bucket=old.started/120000000000 AND state=old.state AND method=old.method AND capture_error=old.capture_error AND failed=old.failed;
 INSERT INTO totals VALUES (new.source,new.state,new.method,new.capture_error,new.failed,1)
 ON CONFLICT(source,state,method,capture_error,failed) DO UPDATE SET n=n+1;
 INSERT INTO buckets VALUES (new.source,new.started/120000000000,new.state,new.method,new.capture_error,new.failed,1)
 ON CONFLICT(source,bucket,state,method,capture_error,failed) DO UPDATE SET n=n+1;
 END;
 CREATE TRIGGER files_insert AFTER INSERT ON files BEGIN
 INSERT INTO settings VALUES ('file_count:'||new.source,1) ON CONFLICT(name) DO UPDATE SET value=value+1;
 END;
 CREATE TRIGGER files_delete AFTER DELETE ON files BEGIN
 UPDATE settings SET value=value-1 WHERE name='file_count:'||old.source;
 END;
