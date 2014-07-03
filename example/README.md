# Colony Example

This example consists of three micro services:
* anthill : produces ants
* anteater: consumes ants and, for whatever reason, emits a bee for every ant it eats. Maybe it is keeping them ransom, who knows.
* honeybadger: consumes bees, and responds to each bee message witha polite "Thankyou"

To run the example you need to build and run each service, and you need to run NSQ. The anthill emits ants every two seconds, and the antater and the honeybadger are pretty instantaneous.

For some insight as to what's going on, you can use nsqadmin to see the state of the topics used to communicate between microservices. 
