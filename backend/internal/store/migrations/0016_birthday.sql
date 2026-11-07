-- Birthday replaces the free-text "age" field.
--
-- Storing an age means the number is wrong the day after the birthday and every
-- year after that, and it asks the person to do date arithmetic. A birth date is
-- entered once and the age is computed on read. `age` is left in place so no
-- existing value is lost; the API simply stops asking for it.
ALTER TABLE users ADD COLUMN birthday TEXT NOT NULL DEFAULT '';  -- ISO YYYY-MM-DD
