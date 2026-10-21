-- Lets the user mark a session as delivered from the chat window. The flag
-- does not change task status and does not recycle the virtual machine.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS user_completed_at timestamp with time zone NULL;
