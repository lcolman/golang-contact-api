# Assumptions

## 1) Database schema choices
I have designed the database with the following in mind:
 1.1. UUID's make sense as unique identifiers for things which may be countably above ~2 million (2^32).
 1.2. Postgres insert performance (not that it matters for a code challenge) on UUID inserts versus BIGSERIAL is (not that much worse for double the identifier space|https://thebuild.com/blog/2015/10/08/uuid-vs-bigserial-for-primary-keys/).
 1.3. Foreign key relationships are not evil, particularly when you're dealing with data that has a natural parent-child relationship (like a contact to a phone number).
 1.4. Enumarated types, whle very databasey, do make sense in certain circumstances, particularly when indexes may be needed and the alternative is a string field or *another* joining table.
 
## 2) Data model choices
The data model used by both the database schema and the HTTP method body logic assumes the following:
 2.1. A name is a name.  If in 2019 you care about "first" and "last" names and don't have some ((much) more elaborate model that takes into account things like culture, family names first, etc|https://www.w3.org/International/questions/qa-personal-names) then don't use my data model!
 2.2. A title is not a name.  If the spec says "The contact data has a name", then a name is what you get.  I'm sure I can add more fields later if it goes into prod. =P
 2.3. Mostly because I'm lazy, my model assumes a given contact can have only one phone number of any given `phone_type` (i.e. work, mobile, etc).  This was just an assumption, made my life easier and whatnot.  In reality I'm sure it does not hold true at all (two work phones, work mobile, etc).
 2.4. An email address is any string with an at "@" symbol in it that has at least one character either side.  Most of the regexes on the web are wrong for email validation anyway (try postmaster@localhost in some of them, etc).

## 3) Phone numbers
Not all phone numbers are created equal.  I have made some assumptions to limit my scope, as phone numbers can get quite complex.
 3.1. A "Phone number" is an ITU-T E.164 numbering plan identifier with at most 15 digits inside it (excluding any international call prefix).
 3.2. A phone *number* doesn't have hyphens, spaces, bracets or any other symbols inside apart from digits.  This means no leading plus "+" sign too.
 3.3. The ITU-T standard says a country code can be up to 3 digits.  I decided to assume the contry code was alrays 3 digits, with 2 and 1 digit codes getting leading zeros.  Australia becomes 061, US becomes 001, etc.
 3.4. As E.164 states a phone number is up to 15 digits long, and up to 12 of these are for the subscriber number, all digits beyond the 3 digit country code are subscriber digits.  I have decided (arbitrarily) that the subscriber number needs to be at least 6 digits long.  Phone numbers over 15 digits are invalid.  I am also ignoring things like extension numbers.