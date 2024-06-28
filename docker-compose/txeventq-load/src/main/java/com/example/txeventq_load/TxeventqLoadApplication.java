// Copyright (c) 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package com.example.txeventq_load;

import java.sql.Connection;
import java.util.Arrays;
import java.util.List;
import java.util.Random;

import javax.sql.DataSource;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.autoconfigure.jms.DefaultJmsListenerContainerFactoryConfigurer;
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.jms.annotation.EnableJms;
import org.springframework.jms.config.DefaultJmsListenerContainerFactory;
import org.springframework.jms.config.JmsListenerContainerFactory;
import org.springframework.jms.core.JmsTemplate;
import org.springframework.jms.support.converter.MappingJackson2MessageConverter;
import org.springframework.jms.support.converter.MessageConverter;
import org.springframework.jms.support.converter.MessageType;

import jakarta.jms.ConnectionFactory;
import jakarta.jms.Destination;
import jakarta.jms.Session;
import jakarta.jms.TopicConnection;
import jakarta.jms.TopicConnectionFactory;
import jakarta.jms.TopicSession;
import lombok.extern.slf4j.Slf4j;
import oracle.jakarta.AQ.AQQueueTableProperty;
import oracle.jakarta.jms.AQjmsDestination;
import oracle.jakarta.jms.AQjmsFactory;
import oracle.jakarta.jms.AQjmsSession;

@SpringBootApplication
@EnableJms
@Slf4j
public class TxeventqLoadApplication implements CommandLineRunner {

	private Random random = new Random();
	private int NUM_TOPICS = 10;

	@Autowired
    private ConfigurableApplicationContext context;

	public static void main(String[] args) {
		SpringApplication.run(TxeventqLoadApplication.class, args);
	}

    @Bean
    public MessageConverter jacksonJmsMessageConverter() {
        MappingJackson2MessageConverter converter = new MappingJackson2MessageConverter();
        converter.setTargetType(MessageType.TEXT);
        converter.setTypeIdPropertyName("_type");
        return converter;
    }


    @Bean
    public JmsTemplate jmsTemplate(ConnectionFactory connectionFactory) {
        JmsTemplate jmsTemplate = new JmsTemplate();
        jmsTemplate.setConnectionFactory(connectionFactory);
        jmsTemplate.setMessageConverter(jacksonJmsMessageConverter());
        return jmsTemplate;
    }

	@Bean
    public JmsListenerContainerFactory<?> factory(ConnectionFactory connectionFactory,
                                                  DefaultJmsListenerContainerFactoryConfigurer configurer) {
        DefaultJmsListenerContainerFactory factory = new DefaultJmsListenerContainerFactory();
        // This provides all boot's default to this factory, including the message converter
        configurer.configure(factory, connectionFactory);
        // You could still override some of Boot's default if necessary.
        return factory;
    }

	@Override
	public void run(String... args) throws Exception {

		JmsTemplate jmsTemplate = (JmsTemplate) context.getBean("jmsTemplate");
		assert (jmsTemplate != null);
		
		// create topics if they don't exist
		for (int i = 0; i < NUM_TOPICS; i++) {
			createTopic(i);
		}

		// send some messages
		while (true) {
			jmsTemplate.convertAndSend("topic_" + random.nextInt(NUM_TOPICS), animals.get(random.nextInt(animals.size())));
			try {
				Thread.sleep(random.nextInt(300));
			} catch (InterruptedException ignore) {}
		}

	}

	public record Animal(
		String name,
		String size
	) {}

	public List<Animal> animals = Arrays.asList(
		new Animal("cat", "small"),
		new Animal("dog", "medium"),
		new Animal("horse", "large"),
		new Animal("elephant", "extra large")
	);

	private void createTopic(int i) {
		DataSource dataSource = (DataSource) context.getBean("dataSource");
		assert (dataSource != null);

		try {
			TopicConnectionFactory tcf = AQjmsFactory.getTopicConnectionFactory(dataSource);
			TopicConnection conn = tcf.createTopicConnection();
			conn.start();
			TopicSession session = (AQjmsSession) conn.createSession(true, Session.AUTO_ACKNOWLEDGE);

			// create properties
			AQQueueTableProperty props = new AQQueueTableProperty("SYS.AQ$_JMS_TEXT_MESAGE");
			props.setMultiConsumer(true);
			props.setPayloadType("SYS.AQ$_JMS_TEXT_MESSAGE");

			// create queue table, topic and start it
			Destination myTeq = ((AQjmsSession) session).createJMSTransactionalEventQueue("topic_" + i, false);
			((AQjmsDestination) myTeq).start(session, true, true);

			// cleanup
			session.close();
			conn.close();

		} catch (Exception e) {
			if (e.getMessage().contains("already exists")) {
				log.info("topic already exists");
			} else {
				log.error("error talking to databsae", e);
			}
		}
	}
}
