#!/usr/bin/env ruby

require "yaml"

MAX_FILES = 256
MAX_FILE_BYTES = 1024 * 1024
MAX_TOTAL_BYTES = 8 * 1024 * 1024
PROTECTED_PUBLISHER_NEEDS = %w[validate-inputs live-preflight assemble-plan].sort.freeze
PROTECTED_RELEASE_NEEDS = {
  "validate-inputs" => [],
  "build-candidate" => %w[validate-inputs],
  "live-preflight" => %w[validate-inputs],
  "assemble-plan" => %w[validate-inputs build-candidate],
  "publish" => PROTECTED_PUBLISHER_NEEDS
}.transform_values { |needs| needs.sort.freeze }.freeze
PROTECTED_PUBLISHER_CONDITION = <<~CONDITION.gsub(/\s+/, " ").strip.freeze
  ${{ inputs.dry_run == false && needs.validate-inputs.result == 'success' && needs.live-preflight.result == 'success' && needs.assemble-plan.result == 'success' && inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}-{4}-{5}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest, needs.validate-inputs.outputs.release_notes_digest, inputs.expected_plan_digest) }}
CONDITION

def fail_policy(path, message)
  warn "#{path}: #{message}"
  false
end

def read_only_permissions?(permissions, require_contents:)
  return false unless permissions.is_a?(Hash)
  normalized = permissions.transform_keys(&:to_s)
  return false if require_contents && normalized["contents"] != "read"

  normalized.values.all? { |value| %w[read none].include?(value.to_s) }
end

def contains_public_release_command?(value)
  case value
  when String
    normalized = value.gsub(/\\[[:space:]]*\n/, " ")
    normalized.match?(/\bgh\s+release\s+(?:upload|create)\b/)
  when Array
    value.any? { |child| contains_public_release_command?(child) }
  when Hash
    value.any? { |key, child| contains_public_release_command?(key) || contains_public_release_command?(child) }
  else
    false
  end
end

def artifact_upload_job?(job)
  return false unless job.is_a?(Hash)
  steps = job["steps"]
  return false unless steps.is_a?(Array)

  steps.any? do |step|
    step.is_a?(Hash) &&
      step["uses"].is_a?(String) &&
      step["uses"].match?(/\Aactions\/upload-artifact@/)
  end
end

def manual_dispatch_only?(triggers)
  case triggers
  when String
    triggers == "workflow_dispatch"
  when Hash
    triggers.keys.map(&:to_s) == ["workflow_dispatch"]
  else
    false
  end
end

def normalized_permissions(permissions)
  return nil unless permissions.is_a?(Hash)

  permissions.transform_keys(&:to_s).transform_values(&:to_s)
end

def write_permissions?(permissions)
  return true if permissions.to_s == "write-all"

  normalized = normalized_permissions(permissions)
  normalized && normalized.values.include?("write")
end

def job_needs(job)
  case job["needs"]
  when String
    [job["needs"]]
  when Array
    job["needs"].map(&:to_s)
  else
    []
  end
end

def protected_release_dependency_graph?(jobs)
  PROTECTED_RELEASE_NEEDS.all? do |name, expected_needs|
    job = jobs[name]
    job.is_a?(Hash) && job_needs(job).sort == expected_needs
  end
end

def protected_publisher?(name, job, jobs)
  return false unless name.to_s == "publish"
  return false unless job.is_a?(Hash)
  return false unless normalized_permissions(job["permissions"]) == { "contents" => "write" }
  return false unless job["environment"] == "protected-release"
  return false if artifact_upload_job?(job)
  return false unless protected_release_dependency_graph?(jobs)

  condition = job["if"]
  return false unless condition.is_a?(String)
  condition.gsub(/\s+/, " ").strip == PROTECTED_PUBLISHER_CONDITION
end

def validate_workflow(path)
  begin
    document = YAML.safe_load(File.read(path), permitted_classes: [], permitted_symbols: [], aliases: false)
  rescue StandardError => error
    return fail_policy(path, "malformed workflow: #{error.message}")
  end
  return fail_policy(path, "malformed workflow: root must be a mapping") unless document.is_a?(Hash)

  jobs = document["jobs"]
  return true unless jobs.is_a?(Hash)
  regulated_jobs = jobs.select do |_name, job|
    next false unless job.is_a?(Hash)

    permissions = job.key?("permissions") ? job["permissions"] : document["permissions"]
    artifact_upload_job?(job) ||
      write_permissions?(permissions) ||
      contains_public_release_command?(job)
  end
  return true if regulated_jobs.empty?

  triggers = document.key?("on") ? document["on"] : document[true]
  return fail_policy(path, "artifact uploads and protected publication require workflow_dispatch-only triggers") unless manual_dispatch_only?(triggers)

  workflow_permissions = document["permissions"]
  unless read_only_permissions?(workflow_permissions, require_contents: true)
    return fail_policy(path, "workflow permissions must be explicit read-only")
  end

  protected_publishers = 0
  jobs.each do |name, job|
    next unless job.is_a?(Hash)

    permissions = job.key?("permissions") ? job["permissions"] : workflow_permissions
    uploads_artifact = artifact_upload_job?(job)
    publishes_release = contains_public_release_command?(job)
    writes = write_permissions?(permissions)
    protected_publisher = protected_publisher?(name, job, jobs)

    if uploads_artifact && !read_only_permissions?(permissions, require_contents: true)
      return fail_policy(path, "job #{name} artifact uploads require contents: read and no writes")
    end

    if writes
      unless protected_publisher
        return fail_policy(path, "job #{name} has forbidden write permissions")
      end
      protected_publishers += 1
    elsif publishes_release
      return fail_policy(path, "job #{name} public release command requires protected publisher")
    end

    if publishes_release && !protected_publisher
      return fail_policy(path, "job #{name} public release command requires protected publisher")
    end
  end
  return fail_policy(path, "multiple protected publisher jobs are forbidden") if protected_publishers > 1

  true
end

if ARGV.empty?
  warn "usage: ci-artifact-upload-policy.rb WORKFLOW..."
  exit 2
end
if ARGV.length > MAX_FILES
  warn "workflow file count limit exceeded"
  exit 1
end

total_bytes = 0
valid = ARGV.all? do |path|
  begin
    stat = File.lstat(path)
  rescue StandardError => error
    fail_policy(path, "workflow cannot be inspected: #{error.message}")
    next false
  end
  if stat.symlink?
    fail_policy(path, "workflow must not be a symlink")
    next false
  end
  if stat.size > MAX_FILE_BYTES
    fail_policy(path, "workflow file size limit exceeded")
    next false
  end
  total_bytes += stat.size
  if total_bytes > MAX_TOTAL_BYTES
    fail_policy(path, "workflow total byte limit exceeded")
    next false
  end
  validate_workflow(path)
end

exit(valid ? 0 : 1)
