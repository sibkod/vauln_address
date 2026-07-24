module mysql

pub struct Config {
pub:
host     string
port     int
user     string
password string
database string
}

pub struct Connection {}

pub fn connect(config Config) !Connection {
return Connection{}
}

pub fn (db Connection) execute(query string) ! {}

pub fn (db Connection) query(query string, params []string) !Rows {
return Rows{}
}

pub struct Rows {}

pub fn (r Rows) next() !bool {
return false
}

pub fn (r Rows) free() {}

pub fn (r Rows) varchar_by_index(i int) !string {
return ''
}

pub fn (r Rows) single_string() !string {
return '0'
}

pub fn (db Connection) close() {}
