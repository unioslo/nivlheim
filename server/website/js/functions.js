function showError(error, domElement, faIconName) {
	$(domElement).html('<nav class="level"><div class="level-left">'
		+'<div class="level-item"><span class="icon">'
		+'<i class="fas fa-lg ' + faIconName + '"></i></span>'
		+'</div><div class="level-item">'+error+'</div></div></nav>');
}

//  http://berzniz.com/post/24743062344/handling-handlebarsjs-like-a-pro
function renderTemplate(name, templateValues, domElement, deferredObj) {
	if (!deferredObj) deferredObj = $.Deferred();
	if (Handlebars.templates === undefined || Handlebars.templates[name] === undefined) {
		// must load and compile the template first
		$.ajax({
			url : 'templates/' + name + '.handlebars',
			dataType : 'text',
			success : function(data) {
				try {
					// compile and keep
					if (Handlebars.templates === undefined) {
						Handlebars.templates = {};
					}
					console.log("Compiling " + name + ".handlebars");
					Handlebars.templates[name] = Handlebars.compile(data, {"strict":true});
					// now, run the template
					var output = Handlebars.templates[name](templateValues);
					$(domElement).html(output);
					deferredObj.resolve(templateValues);
				} catch(err) {
					showError(err, domElement, "fa-exclamation-triangle");
					deferredObj.reject();
				}
			}
		}).fail(function(jqxhr, textStatus){
			showError(jqxhr.status + ' ' + jqxhr.statusCode().responseText,
				domElement, "fa-exclamation-circle");
			deferredObj.reject();
		});
	} else {
		// The template is already compiled
		try {
			var output = Handlebars.templates[name](templateValues);
			$(domElement).html(output);
			deferredObj.resolve(templateValues);
		}
		catch (err) {
			showError(err, domElement, "fa-exclamation-triangle");
			deferredObj.reject();
		}
	}
	return deferredObj.promise();
}

function getAPIURLprefix() {
	if (location.origin.match('http://(127\\.0\\.0\\.1|localhost)')) {
		// Developer mode. Assumes the API is running locally on port 4040.
		return "http://localhost:4040";
	}
	return "";
}

function APIcall(url, templateName, domElement) {
	var deferredObj = $.Deferred();
	$.getJSON(getAPIURLprefix()+url, function(data){
		try {
			renderTemplate(templateName, data, domElement, deferredObj);
		}
		catch(error) {
			showError(error, domElement, "fa-exclamation-triangle");
			deferredObj.reject();
		}
	})
	.fail(function(jqxhr, textStatus){
		if (jqxhr.status == 404)
			showError("404 Not Found", domElement, "fa-unlink");
		else
			showError(jqxhr.status + ' ' + jqxhr.statusCode().responseText,
				domElement, "fa-exclamation-circle");
		deferredObj.reject();
	});
	return deferredObj.promise();
}

function htmlEntities(str) {
	return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;')
		.replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Reads the page's URL parameters and returns them as an associative array
function getUrlParams() {
	var vars = [];
	var start = window.location.href.indexOf('?') + 1;
	if (start == 0) { return vars; }
	var end = window.location.href.indexOf('#');
	if (end < 0) end = window.location.href.length;
	var pairs = window.location.href.slice(start,end).split('&');
	for (var i = 0; i < pairs.length; i++) {
		pair = pairs[i].split('=');
		vars[decodeURIComponent(pair[0])] = decodeURIComponent(pair[1].replace(/\+/g,' '));
	}
	return vars;
}

//----====----====----====-- Frontpage --====----====----====
function approve(id) {
	$.ajax({
		url : getAPIURLprefix()+'/api/v0/awaitingApproval/'
				+id+'?hostname='+$('input#hostname'+id).val(),
		method: "PUT"
	})
	.always(function(){
		APIcall("/api/v0/awaitingApproval"+
				"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
	});
}

function deny(id) {
	$.ajax({
		url : getAPIURLprefix()+'/api/v0/awaitingApproval/'+id,
		method: "DELETE"
	})
	.always(function(){
		APIcall("/api/v0/awaitingApproval"+
				"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
	});
}

//----====----====----====-- Browse hosts and files --====----====----====
function showDiff(data) {
	// We got the first file, let's get the second one and compare
	// Find the ID of the previous version
	let otherFileId = 0;
	for (var i=0; i<data.versions.length-1; i++) {
		if (data.versions[i].fileId == data.fileId) {
			otherFileId = data.versions[i+1].fileId;
			break;
		}
	}
	if (otherFileId == 0) {
		// Don't have any previous version to compare with.
		return;
	}
	// Retrieve the contents of the previous version
	$.getJSON(getAPIURLprefix()+"/api/v0/file?fileId="+otherFileId+"&fields=content",
		function(data2){
			$("div.filecontent").html(diffString(
				htmlEntities(data2.content),
				htmlEntities(data.content)));
		})
		.fail(function(jqxhr, textStatus){
			console.log(jqxhr.status + ' ' + jqxhr.statusCode().responseText);
		});
}
