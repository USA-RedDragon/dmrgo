# Lamp

This page covers every marker kind.

<!-- dmrgo:config pkg=machine type=Config -->
<!-- /dmrgo:config -->

The default wait is <!-- dmrgo:const pkg=machine name=DefaultWait --><!-- /dmrgo:const --> and at most <!-- dmrgo:const pkg=machine name=MaxTries --><!-- /dmrgo:const --> tries.

<!-- dmrgo:states pkg=machine type=Lamp -->
<!-- /dmrgo:states -->

<!-- dmrgo:states pkg=machine type=Counter -->
<!-- /dmrgo:states -->

<!-- dmrgo:errors pkg=machine type=Lamp -->
<!-- /dmrgo:errors -->

<!-- dmrgo:states pkg=machine type=SideState -->
<!-- /dmrgo:states -->

The period is <!-- dmrgo:const pkg=machine name=period --><!-- /dmrgo:const --> and the hold <!-- dmrgo:const pkg=machine name=hold --><!-- /dmrgo:const -->.

<!-- dmrgo:doc pkg=machine section=Rates -->
<!-- /dmrgo:doc -->

<!-- dmrgo:doc pkg=machine name=Lamp.Start -->
<!-- /dmrgo:doc -->

<!-- dmrgo:code file=v2/machine/machine.go lines=96-102 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=machine.ExampleLamp -->
<!-- /dmrgo:output -->

<!-- dmrgo:output example=hello -->
<!-- /dmrgo:output -->

<!-- dmrgo:code file=v2/machine/example_test.go symbol=ExampleLamp -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/machine/machine.go symbol=Lamp.Tick -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/machine/machine.go symbol=SideState -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/machine/machine.go symbol=SideB -->
<!-- /dmrgo:code -->

It needs Go <!-- dmrgo:gomod field=go --><!-- /dmrgo:gomod --> or later.

<!-- dmrgo:flags cmd=cmd/tool -->
<!-- /dmrgo:flags -->

<!-- dmrgo:doc pkg=cmd/tool section=Flags -->
<!-- /dmrgo:doc -->

<!-- dmrgo:code file=v2/examples/hello/main.go -->
<!-- /dmrgo:code -->
