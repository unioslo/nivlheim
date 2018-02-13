$(document).ready(function(){
	p = getUrlParams();
	$('input#search').val(p['q']);

	APIcall("mockapi/systemstatus_data.json", "systemstatus",
		$('#placeholder_systemstatus'));
//	APIcall("mockapi/awaiting_approval.json", "awaiting_approval",
//		$('#placeholder_approval'));
	APIcall("http://127.0.0.1:4040/api/v0/awaitingApproval?fields=hostname,reversedns,ipaddress", "awaiting_approval",
		$('#placeholder_approval'));
	APIcall("mockapi/latestnewmachines.json", "latestnewmachines",
		$('#placeholder_latestnewmachines'));
});
