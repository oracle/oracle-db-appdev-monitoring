-- Copyright (c) 2022, Oracle and/or its affiliates.
-- Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

--
--  This sample demonstrates how to enqueue a message onto a TEQ using PL/SQL
--

--  There are various payload types supported, including user-defined object, raw, JMS and JSON.
--  This sample uses the JMS payload type.

--  Execute permission on dbms_aq is required.

declare
    enqueue_options    dbms_aq.enqueue_options_t;
    message_properties dbms_aq.message_properties_t;
    message_handle     raw(16);
    message            SYS.AQ$_JMS_TEXT_MESSAGE;
begin
    -- create the message payload
    message := SYS.AQ$_JMS_TEXT_MESSAGE.construct;
    message.set_text('{"orderid": 12350, "username": "Jane Smith"}');

    -- set the consumer name 
    message_properties.correlation := 'my_subscriber';

    -- enqueue the message
    dbms_aq.enqueue(
        queue_name           => 'my_teq',           
        enqueue_options      => enqueue_options,       
        message_properties   => message_properties,     
        payload              => message,               
        msgid                => message_handle);
    
    -- commit the transaction
    commit;
end;
/