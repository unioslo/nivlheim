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
					showError(err, domElement, "fa-exclamation");
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

function APIcall(url, templateName, domElement, transform) {
	let origurl = url;
	if (url.startsWith("/api/"))
		url = getAPIURLprefix() + url;
	var deferredObj = $.Deferred();
	$.getJSON(url, function(data){
		try {
			$(domElement).attr({
				"data-api-url": origurl,
				"data-handlebars-template": templateName
			});
			if (typeof transform == 'function') {
				data = transform(data);
			}
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

function htmlEscape(str) {
	return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;')
		.replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, "&#039;");
}

function shuffleArray(array) {
    for (let i = array.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [array[i], array[j]] = [array[j], array[i]]; // eslint-disable-line no-param-reassign
    }
}

// Reads the page's URL parameters and returns them as an associative array
function getUrlParams() {
	var vars = [];
	var start = window.location.href.indexOf('?') + 1;
	if (start == 0) { return vars; }
	var end = window.location.href.indexOf('#');
	if (end < start) end = -1; // if the ? is behind the #
	if (end < 0) end = window.location.href.length;
	var pairs = window.location.href.slice(start,end).split('&');
	for (var i = 0; i < pairs.length; i++) {
		pair = pairs[i].split('=');
		if (pair[0] && pair[1]) {
			vars[decodeURIComponent(pair[0])] = decodeURIComponent(pair[1].replace(/\+/g,' '));
		}
	}
	return vars;
}

function scrollHandler() {
	var docViewTop = $(window).scrollTop();
	var docViewBottom = docViewTop + $(window).height();
	$(".loadmore").each(function(i,elem){
		var elemTop = $(elem).offset().top;
		var elemBottom = elemTop + $(elem).height();
		if (elemTop < docViewBottom && elemBottom > docViewTop) {
			$(elem).detach().scroll().remove();
		}
	});
}

function validateIPv4cidr(addr) {
	// http://stackoverflow.com/questions/5284147/validating-ipv4-addresses-with-regexp
	let re = /^\s*((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(3[012]|[12]?[0-9])\s*$/;
	return addr.match(re);
}

function validateIPv6cidr(addr) {
	// http://stackoverflow.com/questions/53497/regular-expression-that-matches-valid-ipv6-addresses
	let re = /^\s*([0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{1,4}\/(12[0-8]|1[01][0-9]|[0-9]{1,2})\s*$/;
	return addr.match(re);
}

function attachHandlersToForms() {
	$("input[type='text']").change(function(){
		if ($(this).is('.iprange')) {
			$(this).toggleClass('is-danger',
				!(validateIPv4cidr($(this).val()) ||
				  validateIPv6cidr($(this).val()) ||
				  $(this).val().match(/^\s*$/)));
		} else {
			$(this).toggleClass('is-danger', $(this).is(':invalid'));
		}
	});
	$("form").submit(submitForm);
	$(".editbutton").click(editInPlace);
	$(".deletebutton").click(askDeleteAndRefresh);
}

function submitForm(event) {
	// prevent the browser from loading the whole page
	event.preventDefault();
	// replace the submit button with a spinner
	let b = $(event.target).find("input[type=submit]");
	b.replaceWith(
		'<a class="button is-loading" style="width:'+b.width()+'px">Loading</a>');
	// Use the ACTION attribute from the FORM tag
	let path = (new URL(this.action).pathname);
	// use the METHOD or data-method attribute
	let method = this.dataset["method"] || this.method;
	// Serialize the form values
	let data = $(this).serialize();
	// Perform the HTTP request
	AJAXwithRefresh(event.target, path, method, data);
}

function AJAXwithRefresh(domElement, urlPath, method, data) {
	// Perform the HTTP request
	$.ajax({
		"url": getAPIURLprefix()+urlPath,
		"method": method, // Using the METHOD attribute from the FORM tag
		"data": data,
		"processData": false, // Tell jQuery that the data is already encoded
	})
	.fail(function(jqxhr){
		// Error. Display error messages, if any
		let text = jqxhr.statusCode().responseText;
		try {
			// Error messages next to input fields
			let obj = JSON.parse(text);
			for (let prop in obj) {
				if (!obj.hasOwnProperty(prop)) continue;
				$(domElement).find("[data-error-for='"+prop+"']").html(obj[prop]);
			}
		} catch (e) {
			// Generic error message
			alert(text);
		}
	})
	.done(function(data,textStatus,jqxhr){
		// Success. Find the outer placeholder container
		let container = $(domElement).parents("[data-api-url]");
		if (container.length == 0) {
			console.log("Couldn't find container to refresh.");
			return;
		}
		// Make an API call to refresh the appropriate part of the page
		APIcall(container.data("apiUrl"),
			container.data("handlebarsTemplate"),
			"#"+container.attr("id"))
		.done(attachHandlersToForms);
	});
}

function refresh(domElement) {
	// Find the outer placeholder container
	let container = $(domElement).parents("[data-api-url]");
	if (container.length == 0) {
		console.log("Couldn't find the container to refresh.");
		return;
	}
	// Make an API call to refresh the appropriate part of the page
	APIcall(container.data("apiUrl"), container.data("handlebarsTemplate"),
		"#"+container.attr("id"))
	.done(attachHandlersToForms);
}

function editInPlace() {
	// this = the button that was clicked
	let button = this;
	let container = $(this).parents("[data-edit-action]");
	// for each text that should be converted to an input field
	$(container).find("[data-name]").each(function(){
		// in here, "this" is the element with the data-name attribute
		let name = $(this).data("name");
		let value = htmlEscape($(this).text());
		$(this).replaceWith('<input class="input" type="text" '+
			'name="'+name+'" value="'+value+'" '+
			'style="width:'+($(this).width()+30)+'px">');
	});
	// replace the "edit" button with two "accept" and "cancel" buttons
	$(button).replaceWith('<button class="button submit"><i class="fas fa-check color-approve"></i></button>'+
		'<button class="button cancel"><i class="fas fa-times color-deny"></i></button>');
	// add click handlers to the buttons
	$(container).find("button.submit").click(function(event){
		let action = $(container).data("edit-action");
		let body = $(container).find("input").serialize();
		$(event.currentTarget).addClass("is-loading");
		$(container).find("button.cancel").prop("disabled","disabled");
		AJAXwithRefresh(container, action, "PUT", body);
	});
	$(container).find("button.cancel").click(function(){
		refresh(container);
	});
}

function askDeleteAndRefresh() {
	// this = the button that was clicked
	let button = this;
	let container = $(this).parents("[data-edit-action]");
	let name = $(container).find("[data-name=name]").text();
	if (!name) name = $(container).find("[data-name]:first").text();
	// confirm
	if (!confirm("Delete \"" + name + "\", are you sure?")) {
		return;
	}
	let action = $(container).data("edit-action");
	AJAXwithRefresh(container, action, "DELETE");
}

function restPut(apiPath, name, body) {
	let url = getAPIURLprefix()+apiPath+"/"+name;
	$.ajax({
		"url": url,
		"method": "PUT",
		"data": body
	})
	.fail(function(jqxhr){
		// Error. Display error messages, if any
		let text = jqxhr.statusCode().responseText;
		$("[data-error-for='"+name+"']").text(text);
	})
	.done(function(data,textStatus,jqxhr){
		$("[data-error-for='"+name+"']").text('');
		$("[data-saved-for='"+name+"']").text('Saved').show().fadeOut(1000);
	});
}

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

//----====----====----====-- Frontpage --====----====----====
let reloadingTimeout = 0;

function autoReloadStatus() {
	if ($("#placeholder_systemstatus").length == 0) {
		// We're longer on the status page, stop asking the API for status
		return;
	}
	let start = new Date().getTime();
	APIcall(
		//"mockapi/systemstatus_data.json",
		"/api/v0/status",
		"systemstatus",	$('#placeholder_systemstatus'))
		.done(function(data){
			let end = new Date().getTime();
			$("#statusLoadedIn").html("Loaded in "+(end-start)+" ms.");
			// Error messages next to data
			for (let prop in data.errors) {
				$("[data-error-for='"+prop+"']").text(data.errors[prop]);
			}
			// set timeout for next call
			if (reloadingTimeout) window.clearTimeout(reloadingTimeout);
			reloadingTimeout = window.setTimeout(autoReloadStatus, 8000);
		});
	APIcall(
		//"mockapi/latestnewmachines.json",
		"/api/v0/hostlist?fields=hostname,certfp,lastseen"+
			"&sort=-lastseen&limit=20",
		"latestnewmachines", $('div#latestmachines'));
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
				htmlEscape(data2.content),
				htmlEscape(data.content)));
		})
		.fail(function(jqxhr, textStatus){
			console.log(jqxhr.status + ' ' + jqxhr.statusCode().responseText);
		});
}
