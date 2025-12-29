// Unless explicitly stated otherwise all files in this repository are
// dual-licensed under the Apache-2.0 License or BSD-3-Clause License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

#ifndef DDWAF_H
#define DDWAF_H

#ifdef __cplusplus
namespace ddwaf{
class waf;
class context_wrapper;
class waf_builder;
} // namespace ddwaf
using ddwaf_handle = ddwaf::waf *;
using ddwaf_context = ddwaf::context_wrapper *;
using ddwaf_builder = ddwaf::waf_builder *;

extern "C"
{
#endif

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define DDWAF_MAX_STRING_LENGTH 4096
#define DDWAF_MAX_CONTAINER_DEPTH 20
#define DDWAF_MAX_CONTAINER_SIZE 256
#define DDWAF_RUN_TIMEOUT 5000

/**
 * @enum DDWAF_OBJ_TYPE
 *
 * Specifies the type of a ddwaf::object.
 **/
typedef enum
{
    DDWAF_OBJ_INVALID     = 0,
    // 64-bit signed integer type
    DDWAF_OBJ_SIGNED   = 1 << 0,
    // 64-bit unsigned integer type
    DDWAF_OBJ_UNSIGNED = 1 << 1,
    // UTF-8 string of length nbEntries
    DDWAF_OBJ_STRING   = 1 << 2,
    // Array of ddwaf_object of length nbEntries, each item having no parameterName
    DDWAF_OBJ_ARRAY    = 1 << 3,
    // Array of ddwaf_object of length nbEntries, each item having a parameterName
    DDWAF_OBJ_MAP      = 1 << 4,
    // Boolean type
    DDWAF_OBJ_BOOL     = 1 << 5,
    // 64-bit float (or double) type
    DDWAF_OBJ_FLOAT    = 1 << 6,
    // Null type, only used for its semantical value
    DDWAF_OBJ_NULL    = 1 << 7,
} DDWAF_OBJ_TYPE;

/**
 * @enum DDWAF_RET_CODE
 *
 * Codes returned by ddwaf_run.
 **/
typedef enum
{
    DDWAF_ERR_INTERNAL         = -3,
    DDWAF_ERR_INVALID_OBJECT   = -2,
    DDWAF_ERR_INVALID_ARGUMENT = -1,
    DDWAF_OK                   = 0,
    DDWAF_MATCH                = 1,
} DDWAF_RET_CODE;

/**
 * @enum DDWAF_LOG_LEVEL
 *
 * Internal WAF log levels, to be used when setting the minimum log level and cb.
 **/
typedef enum
{
    DDWAF_LOG_TRACE,
    DDWAF_LOG_DEBUG,
    DDWAF_LOG_INFO,
    DDWAF_LOG_WARN,
    DDWAF_LOG_ERROR,
    DDWAF_LOG_OFF,
} DDWAF_LOG_LEVEL;

#ifndef __cplusplus
typedef struct _ddwaf_handle* ddwaf_handle;
typedef struct _ddwaf_context* ddwaf_context;
typedef struct _ddwaf_builder* ddwaf_builder;
#endif

typedef struct _ddwaf_object ddwaf_object;
typedef struct _ddwaf_config ddwaf_config;
/**
 * @struct ddwaf_object
 *
 * Generic object used to pass data and rules to the WAF.
 **/
struct _ddwaf_object
{
    const char* parameterName;
    uint64_t parameterNameLength;
    // uintValue should be at least as wide as the widest type on the platform.
    union
    {
        const char* stringValue;
        uint64_t uintValue;
        int64_t intValue;
        ddwaf_object* array;
        bool boolean;
        double f64;
    };
    uint64_t nbEntries;
    DDWAF_OBJ_TYPE type;
};

/**
 * @typedef ddwaf_object_free_fn
 *
 * Type of the function to free ddwaf::objects.
 **/
typedef void (*ddwaf_object_free_fn)(ddwaf_object *object);

/**
 * @struct ddwaf_config
 *
 * Configuration to be provided to the WAF
 **/
struct _ddwaf_config
{
    struct _ddwaf_config_limits {
        /** Maximum size of ddwaf::object containers. */
        uint32_t max_container_size;
        /** Maximum depth of ddwaf::object containers. */
        uint32_t max_container_depth;
        /** Maximum length of ddwaf::object strings. */
        uint32_t max_string_length;
    } limits;

    /** Obfuscator regexes - the strings are owned by the caller */
    struct _ddwaf_config_obfuscator {
        /** Regular expression for key-based obfuscation */
        const char *key_regex;
        /** Regular expression for value-based obfuscation */
        const char *value_regex;
    } obfuscator;

    /** Function to free the ddwaf::object provided to the context during calls
     *  to ddwaf_run. If the value of this function is NULL, the objects will
     *  not be freed. The default value should be ddwaf_object_free. */
    ddwaf_object_free_fn free_fn;
};

/**
 * @typedef ddwaf_log_cb
 *
 * Callback that powerwaf will call to relay messages to the binding.
 *
 * @param level The logging level.
 * @param function The native function that emitted the message. (nonnull)
 * @param file The file of the native function that emmitted the message. (nonnull)
 * @param line The line where the message was emmitted.
 * @param message The size of the logging message. NUL-terminated
 * @param message_len The length of the logging message (excluding NUL terminator).
 */
typedef void (*ddwaf_log_cb)(
    DDWAF_LOG_LEVEL level, const char* function, const char* file, unsigned line,
    const char* message, uint64_t message_len);

/**
 * ddwaf_init
 *
 * Initialize a ddwaf instance
 *
 * @param ruleset ddwaf::object map containing rules, exclusions, rules_override and rules_data. (nonnull)
 * @param config Optional configuration of the WAF. (nullable)
 * @param diagnostics Optional ruleset parsing diagnostics. (nullable)
 *
 * @return Handle to the WAF instance or NULL on error.
 *
 * @note If config is NULL, default values will be used, including the default
 *       free function (ddwaf_object_free).
 *
 * @note If ruleset is NULL, the diagnostics object will not be initialised.
 **/
ddwaf_handle ddwaf_init(const ddwaf_object *ruleset,
    const ddwaf_config* config, ddwaf_object *diagnostics);

/**
 * ddwaf_destroy
 *
 * Destroy a WAF instance.
 *
 * @param handle Handle to the WAF instance.
 */
void ddwaf_destroy(ddwaf_handle handle);

/**
 * ddwaf_known_addresses
 *
 * Get an array of known (root) addresses used by rules, exclusion filters and
 * processors. This array contains both required and optional addresses. A more
 * accurate distinction between required and optional addresses is provided
 * within the diagnostics.
 *
 * The memory is owned by the WAF and should not be freed.
 *
 * @param handle Handle to the WAF instance.
 * @param size Output parameter in which the size will be returned. The value of
 *             size will be 0 if the return value is NULL.
 * @return NULL if empty, otherwise a pointer to an array with size elements.
 *
 * @note This function is not thread-safe
 * @note The returned array should be considered invalid after calling ddwaf_destroy
 *       on the handle used to obtain it.
 **/
const char* const* ddwaf_known_addresses(const ddwaf_handle handle, uint32_t *size);
/**
 * ddwaf_known_actions
 *
 * Get an array of all the action types which could be triggered as a result of
 * the current set of rules and exclusion filters.
 *
 * The memory is owned by the WAF and should not be freed.
 *
 * @param handle Handle to the WAF instance.
 * @param size Output parameter in which the size will be returned. The value of
 *             size will be 0 if the return value is NULL.
 * @return NULL if empty, otherwise a pointer to an array with size elements.
 *
 * @note This function is not thread-safe
 * @note The returned array should be considered invalid after calling ddwaf_destroy
 *       on the handle used to obtain it.
 **/
const char *const *ddwaf_known_actions(const ddwaf_handle handle, uint32_t *size);
/**
 * ddwaf_context_init
 *
 * Context object to perform matching using the provided WAF instance.
 *
 * @param handle Handle of the WAF instance containing the ruleset definition. (nonnull)

 * @return Handle to the context instance.
 *
 * @note The WAF instance needs to be valid for the lifetime of the context.
 **/
ddwaf_context ddwaf_context_init(const ddwaf_handle handle);

/**
 * ddwaf_run
 *
 * Perform a matching operation on the provided data
 *
 * @param context WAF context to be used in this run, this will determine the
 *                ruleset which will be used and it will also ensure that
 *                parameters are taken into account across runs (nonnull)
 *
 * @param persistent_data Data on which to perform the pattern matching. This
 *    data will be stored by the context and used across multiple calls to this
 *    function. Once the context is destroyed, the used-defined free function
 *    will be used to free the data provided. Note that the data passed must be
 *    valid until the destruction of the context. The object must be a map of
 *    {string, <value>} in which each key represents the relevant address
 *    associated to the value, which can be of an arbitrary type. This parameter
 *    can be null if ephemeral data is provided.
 *
 * @param ephemeral_data Data on which to perform the pattern matching. This
 *    data will not be cached by the WAF. Matches arising from this data will
 *    also not be cached at any level. The data will be freed at the end of the
 *    call to ddwaf_run. The object must be a map of {string, <value>} in which
 *    each key represents the relevant address associated to the value, which
 *    can be of an arbitrary type. This parameter can be null if persistent data
 *    is provided.
 *
 * @param result (nullable) Object map containing the following items:
 *               - events: an array of the generated events.
 *               - actions: a map of the generated actions in the format:
 *                          {action type: { <parameter map> }, ...}
 *               - duration: an unsigned specifying the total runtime of the
 *                           call in nanoseconds.
 *               - timeout: whether there has been a timeout during the call.
 *               - attributes: a map containing all derived objects in the
 *                             format: {tag, value}
 *               - keep: whether the data contained herein must override any
 *                       transport sampling through the relevant mechanism.
 *               This structure must be freed by the caller and will contain all
 *               specified keys when the value returned by ddwaf_run is either
 *               DDWAF_OK or DDWAF_MATCH and will be empty otherwise.
 * @param timeout Maximum time budget in microseconds.
 *
 * @return Return code of the operation.
 * @error DDWAF_ERR_INVALID_ARGUMENT The context is invalid, the data will not
 *                                   be freed.
 * @error DDWAF_ERR_INVALID_OBJECT The data provided didn't match the desired
 *                                 structure or contained invalid objects, the
 *                                 data will be freed by this function.
 * @error DDWAF_ERR_INTERNAL There was an unexpected error and the operation did
 *                           not succeed. The state of the WAF is undefined if
 *                           this error is produced and the ownership of the
 *                           data is unknown. The result structure will not be
 *                           filled if this error occurs.
 *
 * Notes on addresses:
 * - Within a single run, addresses provided should be unique.
 *   If duplicate persistent addresses are provided:
 *   - Within the same batch, the latest one in the structure will be the one
 *     used for evaluation.
 *  - Within two different batches, the second batch will only use the new data.
 *
 *  Ephemeral addresses are designed to be duplicated across batches, but if
 *  duplicate addresses are provided within the same batch, the latest one seen
 *  will be the one used.
 *
 *  Duplicate addresses of different types (ephemeral, persistent), are not
 *  permitted. An existing address will never be replaced by a duplicate one
 *  of a different type, but it doesn't result in a critical failure. Due to the
 *  nature of ephemerals, an ephemeral address can be replaced in a subsequent
 *  batch by a persistent address, however taking advantage of this is not
 *  recommended and might be explicitly rejected in the future.
 **/
DDWAF_RET_CODE ddwaf_run(ddwaf_context context, ddwaf_object *persistent_data,
    ddwaf_object *ephemeral_data, ddwaf_object *result,  uint64_t timeout);

/**
 * ddwaf_context_destroy
 *
 * Performs the destruction of the context, freeing the data passed to it through
 * ddwaf_run using the used-defined free function.
 *
 * @param context Context to destroy. (nonnull)
 **/
void ddwaf_context_destroy(ddwaf_context context);

/**
 * ddwaf_builder_init
 *
 * Initialize an instace of the waf builder.
 *
 * @param config Optional configuration of the WAF. (nullable)
 *
 * @return Handle to the builer instance or NULL on error.
 *
 * @note If config is NULL, default values will be used, including the default
 *       free function (ddwaf_object_free).
 **/
ddwaf_builder ddwaf_builder_init(const ddwaf_config *config);

/**
 * ddwaf_builder_add_or_update_config
 *
 * Adds or updates a configuration based on the given path, which must be a unique
 * identifier for the provided configuration.
 *
 * @param builder Builder to perform the operation on. (nonnull)
 * @param path A string containing the path of the configuration, this must uniquely identify the configuration. (nonnull)
 * @param path_len The length of the string contained within path.
 * @param config ddwaf::object map containing rules, exclusions, rules_override and rules_data. (nonnull)
 * @param diagnostics Optional ruleset parsing diagnostics. (nullable)
 *
 * @return Whether the operation succeeded (true) or failed (false).
 *
 * @note if any of the arguments are NULL, the diagnostics object will not be initialised.
 * @note The memory associated with the path, config and diagnostics must be freed by the caller.
 * @note This function is not thread-safe.
 **/
bool ddwaf_builder_add_or_update_config(ddwaf_builder builder, const char *path, uint32_t path_len, const ddwaf_object *config, ddwaf_object *diagnostics);

/**
 * ddwaf_builder_remove_config
 *
 * Removes a configuration based on the provided path.
 *
 * @param builder Builder to perform the operation on. (nonnull)
 * @param path A string containing the path of the configuration to be removed. (nonnull)
 * @param path_len The length of the string contained within path.
 *
 * @return Whether the operation succeeded (true) or failed (false).
 *
 * @note The memory associated with the path must be freed by the caller.
 * @note This function is not thread-safe.
 **/
bool ddwaf_builder_remove_config(ddwaf_builder builder, const char *path, uint32_t path_len);

/**
 * ddwaf_builder_build_instance
 *
 * Builds a ddwaf instance based on the current set of configurations.
 *
 * @param builder Builder to perform the operation on. (nonnull)
 *
 * @return Handle to the new WAF instance or NULL if there was an error.
 *
 * @note This function is not thread-safe.
 **/
ddwaf_handle ddwaf_builder_build_instance(ddwaf_builder builder);

/**
 * ddwaf_builder_get_config_paths
 *
 * Provides an array of the currently loaded paths, optionally matching the
 * regex provided in filter. In addition, the count is provided as the return
 * value, allowing paths to be nullptr.
 *
 * @param builder Builder to perform the operation on. (nonnull)
 * @param paths The object in which paths will be returned, as an array of
 *        strings. If NULL, only the count is provided. (nullable)
 * @param filter An optional string regex to filter the provided paths. The
 *        provided regular expression is used unanchored so matches can be found
 *        at any point within the path, any necessary anchors must be explicitly
 *        added to the regex. (nullable).
 * @param filter_len The length of the filter string (or 0 otherwise).
 *
 * @return The total number of configurations loaded or, if provided, the number
 *         of those matching the filter.
 *
 * @note This function is not thread-safe and the memory of the paths object must
 *       be freed by the caller.
 **/
uint32_t ddwaf_builder_get_config_paths(ddwaf_builder builder, ddwaf_object *paths, const char *filter, uint32_t filter_len);

/**
 * ddwaf_builder_destroy
 *
 * Destroy an instance of the builder.
 *
 * @param builder Builder to perform the operation on. (nonnull)
 */
void ddwaf_builder_destroy(ddwaf_builder builder);

/**
 * ddwaf_object_invalid
 *
 * Creates an invalid object.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_invalid(ddwaf_object *object);

/**
 * ddwaf_object_null
 *
 * Creates an null object. Provides a different semantical value to invalid as
 * it can be used to signify that a value is null rather than of an unknown type.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_null(ddwaf_object *object);

/**
 * ddwaf_object_string
 *
 * Creates an object from a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String to initialise the object with, this string will be copied
 *               and its length will be calculated using strlen(string). (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_string(ddwaf_object *object, const char *string);

/**
 * ddwaf_object_stringl
 *
 * Creates an object from a string and its length.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String to initialise the object with, this string will be
 *               copied. (nonnull)
 * @param length Length of the string.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_stringl(ddwaf_object *object, const char *string, size_t length);

/**
 * ddwaf_object_stringl_nc
 *
 * Creates an object with the string pointer and length provided.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String pointer to initialise the object with.
 * @param length Length of the string.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_stringl_nc(ddwaf_object *object, const char *string, size_t length);

/**
 * ddwaf_object_string_from_unsigned
 *
 * Creates an object using an unsigned integer (64-bit). The resulting object
 * will contain a string created using the integer provided.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_string_from_unsigned(ddwaf_object *object, uint64_t value);

/**
 * ddwaf_object_string_from_signed
 *
 * Creates an object using a signed integer (64-bit). The resulting object
 * will contain a string created using the integer provided.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_string_from_signed(ddwaf_object *object, int64_t value);

/**
 * ddwaf_object_unsigned_force
 *
 * Creates an object using an unsigned integer (64-bit). The resulting object
 * will contain an unsigned integer as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_unsigned(ddwaf_object *object, uint64_t value);

/**
 * ddwaf_object_signed_force
 *
 * Creates an object using a signed integer (64-bit). The resulting object
 * will contain a signed integer as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_signed(ddwaf_object *object, int64_t value);

/**
 * ddwaf_object_bool
 *
 * Creates an object using a boolean, the resulting object will contain a
 * boolean as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Boolean to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_bool(ddwaf_object *object, bool value);

/**
 * ddwaf_object_float
 *
 * Creates an object using a double, the resulting object will contain a
 * double as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Double to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_float(ddwaf_object *object, double value);

/**
 * ddwaf_object_array
 *
 * Creates an array object, for sequential storage.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_array(ddwaf_object *object);

/**
 * ddwaf_object_map
 *
 * Creates a map object, for key-value storage.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_map(ddwaf_object *object);

/**
 * ddwaf_object_array_add
 *
 * Inserts an object into an array object.
 *
 * @param array Array in which to insert the object. (nonnull)
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_array_add(ddwaf_object *array, ddwaf_object *object);

/**
 * ddwaf_object_map_add
 *
 * Inserts an object into an map object, using a key.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied and its
 *            length will be calcualted using strlen(key). (nonnull)
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_add(ddwaf_object *map, const char *key, ddwaf_object *object);

/**
 * ddwaf_object_map_addl
 *
 * Inserts an object into an map object, using a key and its length.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied (nonnull)
 * @param length Length of the key.
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_addl(ddwaf_object *map, const char *key, size_t length, ddwaf_object *object);

/**
 * ddwaf_object_map_addl_nc
 *
 * Inserts an object into an map object, using a key and its length, but without
 * creating a copy of the key.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied (nonnull)
 * @param length Length of the key.
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_addl_nc(ddwaf_object *map, const char *key, size_t length, ddwaf_object *object);

/**
 * ddwaf_object_from_json
 *
 * Creates a ddwaf_object from a JSON string. The JSON will be parsed and converted
 * into the appropriate ddwaf_object structure, supporting all JSON types including
 * objects, arrays, strings, numbers, booleans, and null values.
 *
 * @param output Object to populate with the parsed JSON data. (nonnull)
 * @param json_str The JSON string to parse. (nonnull)
 * @param length Length of the JSON string.
 *
 * @return The success or failure of the operation.
 *
 * @note The output object must be freed by the caller using ddwaf_object_free.
 * @note If parsing fails, the output object will be left in an undefined state.
 * @note The provided JSON string is owned by the caller.
 **/
bool ddwaf_object_from_json(ddwaf_object *output, const char *json_str, uint32_t length);

/**
 * ddwaf_object_type
 *
 * Returns the type of the object.
 *
 * @param object The object from which to get the type.
 *
 * @return The object type of DDWAF_OBJ_INVALID if NULL.
 **/
DDWAF_OBJ_TYPE ddwaf_object_type(const ddwaf_object *object);

/**
 * ddwaf_object_size
 *
 * Returns the size of the container object.
 *
 * @param object The object from which to get the size.
 *
 * @return The object size or 0 if the object is not a container (array, map).
 **/
size_t ddwaf_object_size(const ddwaf_object *object);

/**
 * ddwaf_object_length
 *
 * Returns the length of the string object.
 *
 * @param object The object from which to get the length.
 *
 * @return The string length or 0 if the object is not a string.
 **/
size_t ddwaf_object_length(const ddwaf_object *object);

/**
 * ddwaf_object_get_key
 *
 * Returns the key contained within the object.
 *
 * @param object The object from which to get the key.
 * @param length Output parameter on which to return the length of the key,
 *               this parameter is optional / nullable.
 *
 * @return The key of the object or NULL if the object doesn't contain a key.
 **/
const char* ddwaf_object_get_key(const ddwaf_object *object, size_t *length);

/**
 * ddwaf_object_get_string
 *
 * Returns the string contained within the object.
 *
 * @param object The object from which to get the string.
 * @param length Output parameter on which to return the length of the string,
 *               this parameter is optional / nullable.
 *
 * @return The string of the object or NULL if the object is not a string.
 **/
const char* ddwaf_object_get_string(const ddwaf_object *object, size_t *length);

/**
 * ddwaf_object_get_unsigned
 *
 * Returns the uint64 contained within the object.
 *
 * @param object The object from which to get the integer.
 *
 * @return The integer or 0 if the object is not an unsigned.
 **/
uint64_t ddwaf_object_get_unsigned(const ddwaf_object *object);

/**
 * ddwaf_object_get_signed
 *
 * Returns the int64 contained within the object.
 *
 * @param object The object from which to get the integer.
 *
 * @return The integer or 0 if the object is not a signed.
 **/
int64_t ddwaf_object_get_signed(const ddwaf_object *object);

/**
 * ddwaf_object_get_float
 *
 * Returns the float64 (double) contained within the object.
 *
 * @param object The object from which to get the float.
 *
 * @return The float or 0.0 if the object is not a float.
 **/
double ddwaf_object_get_float(const ddwaf_object *object);

/**
 * ddwaf_object_get_bool
 *
 * Returns the boolean contained within the object.
 *
 * @param object The object from which to get the boolean.
 *
 * @return The boolean or false if the object is not a boolean.
 **/
bool ddwaf_object_get_bool(const ddwaf_object *object);

/**
 * ddwaf_object_get_index
 *
 * Returns the object contained in the container at the given index.
 *
 * @param object The container from which to extract the object.
 * @param index The position of the required object within the container.
 *
 * @return The requested object or NULL if the index is out of bounds or the
 *         object is not a container.
 **/
const ddwaf_object* ddwaf_object_get_index(const ddwaf_object *object, size_t index);

/**
 * ddwaf_object_find
 *
 * Returns the object within the given map with a key matching the provided one.
 *
 * @param object The container from which to extract the object.
 * @param key A string representing the key to find.
 * @param length Length of the key.
 *
 * @return The requested object or NULL if the key was not found or the
 *         object is not a container.
 **/
const ddwaf_object* ddwaf_object_find(const ddwaf_object *object, const char *key, size_t length);

/**
 * ddwaf_object_free
 *
 * @param object Object to free. (nonnull)
 **/
void ddwaf_object_free(ddwaf_object *object);

/**
 * ddwaf_get_version
 *
 * Return the version of the library
 *
 * @return version Version string, note that this should not be freed
 **/
const char *ddwaf_get_version();

/**
 * ddwaf_set_log_cb
 *
 * Sets the callback to relay logging messages to the binding
 *
 * @param cb The callback to call, or NULL to stop relaying messages
 * @param min_level The minimum logging level for which to relay messages
 *
 * @return whether the operation succeeded or not
 *
 * @note This function is not thread-safe
 **/
bool ddwaf_set_log_cb(ddwaf_log_cb cb, DDWAF_LOG_LEVEL min_level);

#ifdef __cplusplus
}
#endif /* __cplusplus */

#endif /*DDWAF_H */
