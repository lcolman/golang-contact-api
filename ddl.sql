CREATE TABLE contact (
	name TEXT NOT NULL,
	email TEXT NOT NULL,
	contact_id UUID PRIMARY KEY
);

CREATE TYPE phone_type as ENUM (
	'home',
	'work',
	'mobile',
	'fax'
);

CREATE TABLE contact_phone (
	contact_id UUID REFERENCES contact(contact_id),
	contact_phone_type phone_type,
	number varchar(15) NOT NULL,
    PRIMARY KEY (contact_id, contact_phone_type)
);