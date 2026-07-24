module app

import mysql

// Migrator handles database migrations
pub struct Migrator {
	db &mysql.Connection
}

// Create new Migrator
pub fn new_migrator(db &mysql.Connection) &Migrator {
	return &Migrator{db: db}
}

// Run all migrations
pub fn (m &Migrator) run() ! {
	println('Running migrations...')
	
	queries := get_migrations()
	for i, q in queries {
		println('  Migration ${i + 1}/${queries.len}...')
		m.db.execute(q) or {
			if err.msg().contains('already exists') {
				println('    Already exists, skipping...')
			} else {
				return error('Migration failed: ${err}')
			}
		}
	}
	
	println('Migrations complete!')
}

// Seed demo data
pub fn (m &Migrator) seed_demo_data() ! {
	println('Seeding demo data...')
	
	// Insert demo wallets
	queries := get_demo_insert_sql()
	for i, q in queries {
		m.db.execute(q) or {
			// Ignore duplicate errors
			if err.msg().contains('Duplicate') {
				println('  Wallet already exists, skipping...')
			} else {
				return error('Failed to seed: ${err}')
			}
		}
		println('  Seeded wallet ${i + 1}/${queries.len}')
	}
	
	println('Demo data seeded!')
}

// Check if database exists, create if not
pub fn ensure_database(db_config DBConfig) ! {
	// Connect without database first
	mut conn := mysql.connect(mysql.Config{
		host: db_config.host
		port: db_config.port
		user: db_config.user
		password: db_config.password
	})!
	
	// Create database if not exists
	create_db := "CREATE DATABASE IF NOT EXISTS `${db_config.database}` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
	conn.execute(create_db)!
	conn.close()
	
	println('Database ensured: ${db_config.database}')
}
