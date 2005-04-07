require File.dirname(__FILE__) + '/../abstract_unit'

class TestTest < Test::Unit::TestCase
  class TestController < ActionController::Base
    def set_flash
      flash["test"] = ">#{flash["test"]}<"
    end

    def test_uri
      render_text @request.request_uri
    end
  end

  def setup
    @controller = TestController.new
    @request    = ActionController::TestRequest.new
    @response   = ActionController::TestResponse.new
    ActionController::Routing::Routes.reload
  end

  def teardown
    ActionController::Routing::Routes.reload
  end

  def test_process_without_flash
    process :set_flash
    assert_flash_equal "><", "test"
  end

  def test_process_with_flash
    process :set_flash, nil, nil, { "test" => "value" }
    assert_flash_equal ">value<", "test"
  end

  def test_process_with_request_uri_with_no_params
    process :test_uri
    assert_equal @response.body, "/test_test/test/test_uri"
  end

  def test_process_with_request_uri_with_params
    process :test_uri, :id => 7
    assert_equal @response.body, "/test_test/test/test_uri/7"
  end

  def test_process_with_request_uri_with_params_with_explicit_uri
    @request.set_REQUEST_URI "/explicit/uri"
    process :test_uri, :id => 7
    assert_equal @response.body, "/explicit/uri"
  end
end
