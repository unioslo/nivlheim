function showError(error, domElement, faIconName) {
	$(domElement).html('<nav class="level"><div class="level-left">'
		+'<div class="level-item"><span class="icon">'
		+'<i class="fas fa-lg ' + faIconName + '"></i></span>'
		+'</div><div class="level-item">'+error+'</div></div></nav>');
}

//  http://berzniz.com/post/24743062344/handling-handlebarsjs-like-a-pro
function renderTemplate(name, templateValues, domElement, deferredObj) {
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
}

function APIcall(url, templateName, domElement) {
	if (location.origin.match('http://(127\\.0\\.0\\.1|localhost)')) {
		// Developer mode. Assumes the API is running locally on port 4040.
		url = "http://localhost:4040" + url;
	}
	var deferredObj = $.Deferred();
	$.getJSON(url, function(data){
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
