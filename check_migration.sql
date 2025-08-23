-- Check if recipient tracking tables already exist
USE relay;

-- Check for existing tables
SELECT 
    table_name,
    'EXISTS' as status
FROM information_schema.tables 
WHERE table_schema = 'relay' 
    AND table_name IN ('recipients', 'message_recipients', 'recipient_events', 'recipient_lists', 'recipient_list_members')
ORDER BY table_name;