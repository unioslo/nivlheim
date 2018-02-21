$(document).ready(function(){
	p = getUrlParams();
	$('input#search').val(p['q']);

	Handlebars.registerHelper('formatDateTime', function(s){
		if (!s) return "";
		var t = moment(s);
		return t.fromNow() + ' (' + t.format('D MMM Y HH:mm') + ')';
	});

	APIcall(
		//"mockapi/systemstatus_data.json",
		"http://127.0.0.1:4040/api/v0/status",
		"systemstatus",	$('#placeholder_systemstatus'));
	APIcall(
		//"mockapi/awaiting_approval.json",
		"http://127.0.0.1:4040/api/v0/awaitingApproval"+
		"?fields=hostname,reversedns,ipaddress,approvalId",
		"awaiting_approval", $('#placeholder_approval'));
	APIcall(
		//"mockapi/latestnewmachines.json",
		"http://127.0.0.1:4040/api/v0/hostlist?fields=hostname,certfp,lastseen"+
			"&rsort=lastseen&limit=10",
		"latestnewmachines", $('#placeholder_latestnewmachines'));
});

function approve(id) {
	$.ajax({
		url : 'http://127.0.0.1:4040/api/v0/awaitingApproval/'
				+id+'?hostname='+$('input#hostname'+id).val(),
		method: "PUT"
	})
	.always(function(){
		APIcall("http://127.0.0.1:4040/api/v0/awaitingApproval"+
				"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
	});
}

function deny(id) {
	$.ajax({
		url : 'http://127.0.0.1:4040/api/v0/awaitingApproval/'+id,
		method: "DELETE"
	})
	.always(function(){
		APIcall("http://127.0.0.1:4040/api/v0/awaitingApproval"+
				"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
	});
}
