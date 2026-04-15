# AI Task Instruction

## Information for AI: Best Tools for Reading PDF Files

You said that you have two excellent specialized tools for reading PDF files:

1. `extract_pdfs_full_content` (Recommended for Comprehensive Reading)
* Extracts complete content from PDFs for in-depth analysis
* Supports batch processing (multiple PDFs in one call)
* Maximum 30 pages per task
* Ideal when you need to understand the full document

2. `extract_pdfs_key_info` -> alternatively, I prepared python script: `pdf_extract.py`
* Extracts specific information using targeted prompts
* Perfect for finding particular data points, dates, names, statistics
* Supports batch processing (maximum 10 tasks per call)
* Great for screening/filtering multiple PDFs to identify relevant ones

In this Use Case (Multiple PDFs)
here's the recommended workflow:
1. **First pass** → Use `extract_pdfs_key_info` with a broad prompt like "What are the main topics covered in this document?" to quickly identify which PDFs are relevant
2. **Second pass** → Use `extract_pdfs_full_content` on the PDFs you need to study in depth

**Key Advantages**
| Feature               | Benefit                                      |
|----------------------|----------------------------------------------|
| Batch processing     | Process up to 10 PDFs simultaneously         |
| Targeted extraction  | Ask specific questions, get precise answers  |
| Full content mode    | Deep dive into complete documents            |
| Page range control   | Extract only relevant sections               |

## Execution Strategy

You MUST follow this 2-phase approach:

### Phase 1: Planning
- Analyze the task carefully
- Break down into clear implementation steps
- Identify risks and edge cases
- Output a structured plan BEFORE coding

### Phase 2: Execution
- Implement based on the approved plan
- Write clean, maintainable code
- Add unit tests where applicable
- Ensure code compiles and works correctly

Do NOT skip Phase 1.

## Rules
- be direct
- be consistent

## Execution details

here you will acts multiple roles: system analyst, system architect, database administrator, senior software engineer, senior frontend developer, and senior QA. read this instruction: `_docs/reference-pdf`
inside that folders, I split 128 pages of instruction into 7 chunk of PDF files.
you may summarize information inside that PDF file. First, store your summary in file `_docs/markdown-results/summary.md` and then, divide them into multiple markdown files *.md based on the role and its jobs todo. So far I have generated some markdown files here: `_docs/markdown-results`. you will create new markdown files based on the role and its jobs todo.
each role will read the main pdf file and each newly generated markdown file, then follow the execution strategy below:
