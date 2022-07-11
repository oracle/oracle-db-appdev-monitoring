-- Copyright (c) 2022, Oracle and/or its affiliates.
-- Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

--
--  This sample demonstrates how to create a TEQ using PL/SQL
--

--  There are various payload types supported, including user-defined object, raw, JMS and JSON.
--  This sample uses the JMS payload type (which is the default).

--  Execute permission on dbms_aqadm is required.

begin
    -- create the TEQ
    dbms_aqadm.create_transactional_event_queue(
        -- note, in Oracle 19c this is called create_sharded_queue() but has the same parameters
        queue_name         => 'my_teq',
        -- when mutiple_consumers is true, this will create a pub/sub "topic" - the default is false
        multiple_consumers => true
    );
    
    -- start the TEQ
    dbms_aqadm.start_queue(
        queue_name         => 'my_teq'
    ); 
end;
/

--
--  You may also want to create a subscriber for the TEQ, pub/sub topics normally deliver 
--  messages only when the consumer/subscriber is present. 
--

declare
    subscriber sys.aq$_agent;
begin
    dbms_aqadm.add_subscriber(
        queue_name => 'my_teq',
        subscriber => sys.aq$_agent(
            'my_subscriber',    -- the subscriber name
            null,               -- address, only used for notifications
            0                   -- protocol
        ),
        rule => 'correlation = ''my_subscriber'''
    );
end;
/