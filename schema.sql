CREATE TABLE karma (
	id SERIAL PRIMARY KEY,
	name VARCHAR(512) UNIQUE,
	score INTEGER
);