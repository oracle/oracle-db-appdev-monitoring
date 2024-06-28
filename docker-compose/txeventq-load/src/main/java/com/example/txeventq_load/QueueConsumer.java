// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package com.example.txeventq_load;

import java.util.Random;

import org.springframework.jms.annotation.JmsListener;
import org.springframework.stereotype.Component;

@Component
public class QueueConsumer {

    private Random random = new Random();
    
    @JmsListener(destination = "topic_0", containerFactory = "factory", concurrency = "1")
    public void topic0(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }
    
    @JmsListener(destination = "topic_1", containerFactory ="factory", concurrency = "1")
    public void topic1(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_2", containerFactory ="factory", concurrency = "1")
    public void topic2(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_3", containerFactory ="factory", concurrency = "1")
    public void topic3(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_4", containerFactory ="factory", concurrency = "1")
    public void topic4(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_5", containerFactory ="factory", concurrency = "1")
    public void topic5(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_6", containerFactory ="factory", concurrency = "1")
    public void topic6(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_7", containerFactory ="factory", concurrency = "1")
    public void topic7(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_8", containerFactory ="factory", concurrency = "1")
    public void topic8(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_9", containerFactory ="factory", concurrency = "1")
    public void topic9(Object object) {
        try {
            Thread.sleep(random.nextInt(2000)+500);
        } catch (InterruptedException ignore) {}
    }

}
