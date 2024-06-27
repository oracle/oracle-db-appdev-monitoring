package com.example.txeventq_load;

import java.util.Random;

import org.springframework.jms.annotation.JmsListener;
import org.springframework.stereotype.Component;

@Component
public class QueueConsumer {

    private Random random = new Random();
    
    @JmsListener(destination = "topic_0", containerFactory = "factory")
    public void topic0(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }
    
    @JmsListener(destination = "topic_1", containerFactory = "factory")
    public void topic1(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_2", containerFactory = "factory")
    public void topic2(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_3", containerFactory = "factory")
    public void topic3(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_4", containerFactory = "factory")
    public void topic4(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_5", containerFactory = "factory")
    public void topic5(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_6", containerFactory = "factory")
    public void topic6(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_7", containerFactory = "factory")
    public void topic7(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_8", containerFactory = "factory")
    public void topic8(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

    @JmsListener(destination = "topic_9", containerFactory = "factory")
    public void topic9(Object object) {
        try {
            Thread.sleep(random.nextInt(1000));
        } catch (InterruptedException ignore) {}
    }

}
