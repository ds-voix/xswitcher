#!/usr/bin/perl
# The simplest way to test complex regex against the input.
if ("B:1,B:0,SPACE:1,SPACE:0,L_SHIFT:1,P:1" =~ /((((BACK)?SPACE)|([=-]|(KP)?MINUS|(KP)?ENTER|ESC|TAB)):[12]),(((CAPS:[012])|([LR]_SHIFT:[12])|(R_META:0)|((([0-9A-Z=-]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9])|(((BACK)?SPACE)|([=-]|(KP)?MINUS|(KP)?ENTER|ESC|TAB))):0)),)+(([0-9A-Z=-]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9]):1)$/) {
  print "matched\n";                                                                                                                                                                                                                       #
};
