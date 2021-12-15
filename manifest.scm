(use-modules (ice-9 textual-ports)
             (git bindings)
             (git repository)
             (git reference)
             (guix packages)
             (guix gexp)
             ((guix git-download) #:select (git-predicate))
             (uio packages nivlheim))

(define %checkout (dirname (current-filename)))

(define version
  (call-with-input-file (string-append %checkout "/VERSION")
    (lambda (port)
      (string-trim-right (get-string-all port)))))

(define branch
  (or (getenv "GITHUB_REF_NAME")
      (if (file-exists? (string-append %checkout "/.git"))
          (begin
            (libgit2-init!)
            (let* ((repo (repository-open %checkout))
                   (head (repository-head repo))
                   (branch (reference-shorthand head)))
              (repository-close! repo)
              (libgit2-shutdown!)
              branch))
          "unknown")))


(packages->manifest
 (list (package
         ;; Return a variant of Nivlheim that uses the local checkout as
         ;; source, and with a custom version based on the contents of
         ;; the VERSION file and the current branch.
         (inherit nivlheim)
         (version (if (string=? branch "master")
                      version
                      (string-append version "-" branch)))
         (source (local-file %checkout
                             #:recursive? #t
                             #:select? (git-predicate %checkout))))))
