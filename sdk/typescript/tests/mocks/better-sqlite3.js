// Mock better-sqlite3 for testing — provides an in-memory database implementation

class MockStatement {
  constructor(sql, db) {
    this.sql = sql;
    this.db = db;
  }

  run(...params) {
    if (this.sql.includes('INSERT OR REPLACE')) {
      const id = params[0];
      this.db.data.set(id, {
        agent_id: id,
        session_id: params[1],
        status: params[2],
        messages: params[3],
        turn_count: params[4],
        metrics: params[5],
        saved_at: params[6],
      });
      return { changes: 1 };
    }
    if (this.sql.includes('DELETE')) {
      const id = params[0];
      if (this.db.data.has(id)) {
        this.db.data.delete(id);
        return { changes: 1 };
      }
      return { changes: 0 };
    }
    return { changes: 0 };
  }

  get(...params) {
    const id = params[0];
    return this.db.data.get(id);
  }

  all(...params) {
    const sessionId = params[0];
    const results = [];
    for (const row of this.db.data.values()) {
      if (row.session_id === sessionId) {
        results.push(row);
      }
    }
    return results;
  }
}

class MockDatabase {
  constructor(dbPath) {
    this.dbPath = dbPath;
    this.data = new Map();
    this.closed = false;
  }

  exec(_sql) {
    // No-op for table creation
  }

  pragma(_pragma) {
    // No-op
  }

  prepare(sql) {
    return new MockStatement(sql, this);
  }

  close() {
    this.closed = true;
  }
}

module.exports = MockDatabase;
module.exports.default = MockDatabase;
