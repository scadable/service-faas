# Service FaaS (Function as a Service)

This project is a FaaS (Function as a Service) management layer written in Go. It provides an API to upload, manage, and execute custom Python functions. Each uploaded function is dynamically deployed as an independent, scalable microservice using either Docker or Kubernetes as the backend orchestrator.

# Architecture
This service acts as the central control plane that manages the lifecycle of individual worker functions. The manager receives API requests to add or execute code, and then it communicates with the underlying container orchestrator (Docker or Kubernetes) to run the specific `worker-faas` instances.

<img alt="image" src="https://github.com/user-attachments/assets/467b773f-e792-4dbc-9cc2-6175a4fe4f1f" />


# Features
- **Dynamic Function Deployment:** Upload Python code via a REST API to deploy it as a new, isolated service.
- **Pluggable Orchestration:** Supports both Docker for local development and Kubernetes for scalable production deployments.
- **Scalability:** Automatically creates Kubernetes Deployments, Services, and Horizontal Pod Autoscalers (HPA) for each function, allowing them to scale based on CPU and memory usage.
- **Simple API:** A straightforward HTTP API for adding, listing, executing, and removing functions.
- **Persistent State:** Uses a PostgreSQL database to keep track of all deployed functions.

# Related Projects
- **Worker Implementation:** This service is designed to manage and deploy instances of the [worker-faas](https://github.com/scadable/worker-faas) project. The worker is a generic FastAPI application that loads and runs the custom Python handler.

# Writing a Handler Function
To be compatible with this FaaS system, your Python code must contain a function with a specific signature. The service will upload this file and configure the worker to execute a specific function within it.

Your Python file must contain a function that accepts a single string argument (`payload`) and returns any JSON-serializable value.

### Example handler.py:

~~~Python
from typing import Any
import json


def handle(payload: str) -> Any:
    """
    This is the default handler function.
    It attempts to parse the input as JSON and adds a status field.

    :param payload: The input string from the POST request.
    :return: The processed result.
    """
    print(f"Default handler received: '{payload}'")
    try:
        # Assume the data is a JSON string
        data = json.loads(payload)
        return {
            "handler": "default",
            "processed_data": data,
            "status": "processed_as_json"
        }
    except json.JSONDecodeError:
        # If not JSON, return it as a plain string
        return {
            "handler": "default",
            "original_data": payload,
            "status": "processed_as_string"
        }
~~~



# API Usag
## Add a new function

Uploads a Python file and deploys it as a new function.
- **Endpoint:** `POST /functions`
- **Request Type:** `multipart/form-data`
- **Form Fields:**
  - `python_file`: The Python file containing your handler code.
  - `function_name`: The name of the function to be called inside your Python file (e.g., handle).

### Example cURL Request:

~~~Bash
curl -X POST http://localhost:8080/functions \
  -F "python_file=@/path/to/your/handler.py" \
  -F "function_name=handle"
~~~
## Execute a function

Sends a payload to a deployed function for execution.

- **Endpoint:** `POST /functions/{functionID}/execute`
- **Request Body:** A JSON object with a single payload key containing the string data you want to send to the function.

### Example cURL Request:

~~~Bash
curl -X POST http://localhost:8080/functions/your_function_id/execute \
  -H "Content-Type: application/json" \
  -d '{"payload": "{\"key\": \"some value\"}"}'
~~~
## List all functions

Retrieves a list of all currently managed functions.
- **Endpoint:** `GET /functions`

### Example cURL Request:

~~~Bash
curl http://localhost:8080/functions
~~~
## Remove a function

Stops the function's container/deployment and deletes its data.
-** Endpoint:** `DELETE /functions/{functionID}`

### Example cURL Request:

~~~Bash
curl -X DELETE http://localhost:8080/functions/your_function_id
~~~

**Note:** The repository includes all necessary manifest files to deploy the service and its dependencies to a Kubernetes cluster.
