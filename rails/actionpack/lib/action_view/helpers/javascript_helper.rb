require File.dirname(__FILE__) + '/tag_helper'

module ActionView
  module Helpers
    # Provides a set of helpers for calling Javascript functions and, most importantly, to call remote methods using what has 
    # been labelled Ajax[http://www.adaptivepath.com/publications/essays/archives/000385.php]. This means that you can call 
    # actions in your controllers without reloading the page, but still update certain parts of it using injections into the 
    # DOM. The common use case is having a form that adds a new element to a list without reloading the page.
    #
    # To be able to use the Javascript helpers, you must either call <tt><%= define_javascript_functions %></tt> (which returns all
    # the Javascript support functions in a <script> block) or reference the Javascript library using 
    # <tt><%= javascript_include_tag "prototype" %></tt> (which looks for the library in /javascripts/prototype.js). The latter is
    # recommended as the browser can then cache the library instead of fetching all the functions anew on every request.
    #
    # If you're the visual type, there's an Ajax movie[http://www.rubyonrails.com/media/video/rails-ajax.mov] demonstrating
    # the use of form_remote_tag.
    module JavascriptHelper      
      unless const_defined? :CALLBACKS
        CALLBACKS = [:uninitialized, :loading, :loaded, :interactive, :complete]
        JAVASCRIPT_PATH = File.join(File.dirname(__FILE__), 'javascripts')
      end
      
      # Returns a link that'll trigger a javascript +function+ using the 
      # onclick handler and return false after the fact.
      #
      # Examples:
      #   link_to_function "Greeting", "alert('Hello world!')"
      #   link_to_function(image_tag("delete"), "if confirm('Really?'){ do_delete(); }")
      def link_to_function(name, function, html_options = {})
        content_tag(
          "a", name, 
          html_options.symbolize_keys.merge(:href => "#", :onclick => "#{function}; return false;")
        )
      end

      # Returns a link to a remote action defined by <tt>options[:url]</tt> 
      # (using the url_for format) that's called in the background using 
      # XMLHttpRequest. The result of that request can then be inserted into a
      # DOM object whose id can be specified with <tt>options[:update]</tt>. 
      # Usually, the result would be a partial prepared by the controller with
      # either render_partial or render_partial_collection. 
      #
      # Examples:
      #  link_to_remote "Delete this post", :update => "posts", :url => { :action => "destroy", :id => post.id }
      #  link_to_remote(image_tag("refresh"), :update => "emails", :url => { :action => "list_emails" })
      #
      # By default, these remote requests are processed asynchronous during 
      # which various callbacks can be triggered (for progress indicators and
      # the likes).
      #
      # Example:
      #   link_to_remote word,
      #       :url => { :action => "undo", :n => word_counter },
      #       :complete => "undoRequestCompleted(request)"
      #
      # The callbacks that may be specified are:
      #
      # <tt>:loading</tt>::       Called when the remote document is being 
      #                           loaded with data by the browser.
      # <tt>:loaded</tt>::        Called when the browser has finished loading
      #                           the remote document.
      # <tt>:interactive</tt>::   Called when the user can interact with the 
      #                           remote document, even though it has not 
      #                           finished loading.
      # <tt>:complete</tt>::      Called when the XMLHttpRequest is complete.
      #
      # If you for some reason or another need synchronous processing (that'll
      # block the browser while the request is happening), you can specify 
      # <tt>options[:type] = :synchronous</tt>.
      def link_to_remote(name, options = {}, html_options = {})  
        link_to_function(name, remote_function(options), html_options)
      end

      # Returns a form tag that will submit using XMLHttpRequest in the background instead of the regular 
      # reloading POST arrangement. Even though it's using Javascript to serialize the form elements, the form submission 
      # will work just like a regular submission as viewed by the receiving side (all elements available in @params).
      # The options for specifying the target with :url and defining callbacks is the same as link_to_remote.
      def form_remote_tag(options = {})
        options[:form] = true

        options[:html] ||= { }
        options[:html][:onsubmit] = "#{remote_function(options)}; return false;"

        tag("form", options[:html], true)
      end

      def remote_function(options) #:nodoc: for now
        javascript_options = options_for_ajax(options)

        function = options[:update] ? 
          "new Ajax.Updater('#{options[:update]}', " :
          "new Ajax.Request("

        function << "'#{url_for(options[:url])}'"
        function << ", #{javascript_options})"
        
        function = "#{options[:before]}; #{function}" if options[:before]
        function = "#{function}; #{options[:after]}"  if options[:after]
        function = "if (#{options[:condition]}) { #{function}; }" if options[:condition]

        return function
      end

      # Includes the Action Pack Javascript library inside a single <script> 
      # tag.
      #
      # Note: The recommended approach is to copy the contents of
      # lib/action_view/helpers/javascripts/ into your application's
      # public/javascripts/ directory, and use +javascript_include_tag+ to 
      # create remote <script> links.
      def define_javascript_functions
        javascript = '<script type="text/javascript">'
        Dir.glob(File.join(JAVASCRIPT_PATH, '*')).each do |filename|
          javascript << "\n" << IO.read(filename)
        end
        javascript << '</script>'
      end

      # Observes the field with the DOM ID specified by +field_id+ and makes
      # an Ajax when its contents have changed.
      # 
      # Required +options+ are:
      # <tt>:frequency</tt>:: The frequency (in seconds) at which changes to
      #                       this field will be detected.
      # <tt>:url</tt>::       +url_for+-style options for the action to call
      #                       when the field has changed.
      # 
      # Additional options are:
      # <tt>:update</tt>::    Specifies the DOM ID of the element whose 
      #                       innerHTML should be updated with the
      #                       XMLHttpRequest response text.
      # <tt>:with</tt>::      A Javascript expression specifying the
      #                       parameters for the XMLHttpRequest. This defaults
      #                       to 'value', which in the evaluated context 
      #                       refers to the new field value.
      #
      # Additionally, you may specify any of the options documented in
      # +link_to_remote.
      def observe_field(field_id, options = {})
        build_observer('Form.Element.Observer', field_id, options)
      end
      
      # Like +observe_field+, but operates on an entire form identified by the
      # DOM ID +form_id+. +options+ are the same as +observe_field+, except 
      # the default value of the <tt>:with</tt> option evaluates to the
      # serialized (request string) value of the form.
      def observe_form(form_id, options = {})
        build_observer('Form.Observer', form_id, options)
      end

      # Escape carrier returns and single and double quotes for Javascript segments.
      def escape_javascript(javascript)
        (javascript || '').gsub(/\r\n|\n|\r/, "\\n").gsub(/["']/) { |m| "\\#{m}" }
      end

    private      
      def options_for_ajax(options)
        js_options = build_callbacks(options)
        
        js_options['asynchronous'] = options[:type] != :synchronous
        js_options['method'] = options[:method] if options[:method]
      	js_options['effect'] = ("\'"+options[:effect].to_s+"\'") if options[:effect]        
	
        if options[:form]
          js_options['parameters'] = 'Form.serialize(this)'
        elsif options[:with]
          js_options['parameters'] = options[:with]
        end
        
        '{' + js_options.map {|k, v| "#{k}:#{v}"}.join(', ') + '}'
      end
      
      def build_observer(klass, name, options = {})
        options[:with] ||= 'value' if options[:update]
        callback = remote_function(options)
        javascript = '<script type="text/javascript">'
        javascript << "new #{klass}('#{name}', "
        javascript << "#{options[:frequency]}, function(element, value) {"
        javascript << "#{callback}})</script>"
      end
            
      def build_callbacks(options)
        CALLBACKS.inject({}) do |callbacks, callback|
          if options[callback]
            name = 'on' + callback.to_s.capitalize
            code = escape_javascript(options[callback])
            callbacks[name] = "function(request){#{code}}"
          end
          callbacks
        end
      end
    end
  end
end
