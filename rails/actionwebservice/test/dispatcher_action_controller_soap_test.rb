$:.unshift(File.dirname(__FILE__) + '/apis')
require File.dirname(__FILE__) + '/abstract_dispatcher'
require 'wsdl/parser'

class ActionController::Base
  class << self
    alias :inherited_without_name_error :inherited
    def inherited(child)
      begin
        inherited_without_name_error(child)
      rescue NameError => e
      end
    end
  end
end

class AutoLoadController < ActionController::Base; end
class FailingAutoLoadController < ActionController::Base; end
class BrokenAutoLoadController < ActionController::Base; end

class TC_DispatcherActionControllerSoap < Test::Unit::TestCase
  include DispatcherTest
  include DispatcherCommonTests

  def setup
    @encoder = WS::Encoding::SoapRpcEncoding.new 'urn:ActionWebService'
    @marshaler = WS::Marshaling::SoapMarshaler.new 'urn:ActionWebService'
    @direct_controller = DirectController.new
    @delegated_controller = DelegatedController.new
    @virtual_controller = VirtualController.new
  end

  def test_wsdl_generation
    ensure_valid_wsdl_generation DelegatedController.new
    ensure_valid_wsdl_generation DirectController.new
  end

  def test_wsdl_action
    delegated_types = ensure_valid_wsdl_action DelegatedController.new
    delegated_names = delegated_types.map{|x| x.name.name}
    assert(delegated_names.include?('DispatcherTest..NodeArray'))
    assert(delegated_names.include?('DispatcherTest..Node'))
    direct_types = ensure_valid_wsdl_action DirectController.new
    direct_names = direct_types.map{|x| x.name.name}
    assert(direct_names.include?('DispatcherTest..NodeArray'))
    assert(direct_names.include?('DispatcherTest..Node'))
    assert(direct_names.include?('IntegerArray'))
  end

  def test_autoloading
    assert(!AutoLoadController.web_service_api.nil?)
    assert(AutoLoadController.web_service_api.has_public_api_method?('Void'))
    assert(FailingAutoLoadController.web_service_api.nil?)
    assert_raises(MissingSourceFile) do
      FailingAutoLoadController.require_web_service_api :blah
    end
    assert_raises(ArgumentError) do
      FailingAutoLoadController.require_web_service_api 50.0
    end
    assert(BrokenAutoLoadController.web_service_api.nil?)
  end

  protected
    def exception_message(soap_fault_exception)
      soap_fault_exception.detail.cause.message
    end

    def is_exception?(obj)
      obj.respond_to?(:detail) && obj.detail.respond_to?(:cause) && \
      obj.detail.cause.is_a?(Exception)
    end

    def create_ap_request(container, body, public_method_name, *args)
      test_request = ActionController::TestRequest.new
      test_request.request_parameters['action'] = service_name(container)
      test_request.env['REQUEST_METHOD'] = "POST"
      test_request.env['HTTP_CONTENT_TYPE'] = 'text/xml'
      test_request.env['HTTP_SOAPACTION'] = "/soap/#{service_name(container)}/#{public_method_name}"
      test_request.env['RAW_POST_DATA'] = body
      test_request
    end

    def service_name(container)
      container.is_a?(DelegatedController) ? 'test_service' : 'api'
    end

    def ensure_valid_wsdl_generation(controller)
      wsdl = controller.generate_wsdl
      ensure_valid_wsdl(wsdl)
    end

    def ensure_valid_wsdl(wsdl)
      definitions = WSDL::Parser.new.parse(wsdl)
      assert(definitions.is_a?(WSDL::Definitions))
      definitions.bindings.each do |binding|
        assert(binding.name.name.index(':').nil?)
      end
      definitions.services.each do |service|
        service.ports.each do |port|
          assert(port.name.name.index(':').nil?)
        end
      end
      types = definitions.collect_complextypes.map{|x| x.name}
      types.each do |type|
        assert(type.namespace == 'urn:ActionWebService')
      end
      definitions.collect_complextypes
    end

    def ensure_valid_wsdl_action(controller)
      test_request = ActionController::TestRequest.new({ 'action' => 'wsdl' })
      test_request.env['REQUEST_METHOD'] = 'GET'
      test_request.env['HTTP_HOST'] = 'localhost:3000'
      test_response = ActionController::TestResponse.new
      wsdl = controller.process(test_request, test_response).body
      ensure_valid_wsdl(wsdl)
    end
end
